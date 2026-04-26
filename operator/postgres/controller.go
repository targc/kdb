package postgres

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pg := &Postgres{}
	if err := r.Get(ctx, req.NamespacedName, pg); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get Postgres: %w", err)
	}

	image := pg.Spec.Image
	if image == "" {
		image = "postgres:16"
	}

	for _, fn := range []func() error{
		func() error { return r.reconcilePVC(ctx, pg) },
		func() error { return r.reconcileTLSOption(ctx, pg) },
		func() error { return r.reconcileIngressRouteTCP(ctx, pg) },
		func() error { return r.reconcileService(ctx, pg) },
		func() error { return r.reconcileDeployment(ctx, pg, image) },
	} {
		if err := fn(); err != nil {
			if errors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			r.setPhase(ctx, pg, "Error")
			return ctrl.Result{}, err
		}
	}
	r.setPhase(ctx, pg, "Running")
	return ctrl.Result{}, nil
}

func (r *Reconciler) setPhase(ctx context.Context, pg *Postgres, phase string) {
	pg.Status.Phase = phase
	if err := r.Status().Update(ctx, pg); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "failed to update status", "phase", phase)
	}
}

func (r *Reconciler) reconcilePVC(ctx context.Context, pg *Postgres) error {
	storageClass := pg.Spec.Storage.StorageClass
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: pg.Spec.Storage.PVCName, Namespace: pg.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		if err := controllerutil.SetControllerReference(pg, pvc, r.Scheme); err != nil {
			return err
		}
		if pvc.CreationTimestamp.IsZero() {
			modes := make([]corev1.PersistentVolumeAccessMode, len(pg.Spec.Storage.AccessModes))
			for i, m := range pg.Spec.Storage.AccessModes {
				modes[i] = corev1.PersistentVolumeAccessMode(m)
			}
			pvc.Spec.AccessModes = modes
			pvc.Spec.StorageClassName = &storageClass
			pvc.Spec.Resources = corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(pg.Spec.Storage.Size),
				},
			}
		}
		return nil
	})
	return wrap("PVC", err)
}

func (r *Reconciler) reconcileTLSOption(ctx context.Context, pg *Postgres) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "traefik.io", Version: "v1alpha1", Kind: "TLSOption"})
	obj.SetName(pg.Name + "-tls")
	obj.SetNamespace(pg.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
		if err := controllerutil.SetControllerReference(pg, obj, r.Scheme); err != nil {
			return err
		}
		obj.Object["spec"] = map[string]interface{}{
			"alpnProtocols": []interface{}{"postgresql"},
		}
		return nil
	})
	return wrap("TLSOption", err)
}

func (r *Reconciler) reconcileIngressRouteTCP(ctx context.Context, pg *Postgres) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "traefik.io", Version: "v1alpha1", Kind: "IngressRouteTCP"})
	obj.SetName(pg.Name)
	obj.SetNamespace(pg.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
		if err := controllerutil.SetControllerReference(pg, obj, r.Scheme); err != nil {
			return err
		}
		obj.Object["spec"] = map[string]interface{}{
			"entryPoints": []interface{}{"tcp"},
			"routes": []interface{}{
				map[string]interface{}{
					"match": fmt.Sprintf("HostSNI(`%s`)", pg.Spec.Domain),
					"services": []interface{}{
						map[string]interface{}{"name": pg.Name, "port": int64(5432)},
					},
				},
			},
			"tls": map[string]interface{}{
				"secretName": "tls-cert",
				"options": map[string]interface{}{
					"name":      pg.Name + "-tls",
					"namespace": pg.Namespace,
				},
			},
		}
		return nil
	})
	return wrap("IngressRouteTCP", err)
}

func (r *Reconciler) reconcileService(ctx context.Context, pg *Postgres) error {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: pg.Name, Namespace: pg.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if err := controllerutil.SetControllerReference(pg, svc, r.Scheme); err != nil {
			return err
		}
		svc.Spec.Selector = map[string]string{"app": pg.Name}
		svc.Spec.Ports = []corev1.ServicePort{
			{Name: "postgres", Port: 5432, TargetPort: intstr.FromInt32(5432)},
		}
		return nil
	})
	return wrap("Service", err)
}

func (r *Reconciler) reconcileDeployment(ctx context.Context, pg *Postgres, image string) error {
	replicas := int32(1)
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: pg.Name, Namespace: pg.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		if err := controllerutil.SetControllerReference(pg, dep, r.Scheme); err != nil {
			return err
		}
		labels := map[string]string{"app": pg.Name}
		if dep.CreationTimestamp.IsZero() {
			dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		}
		dep.Spec.Replicas = &replicas
		dep.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name:  "postgres",
					Image: image,
					Ports: []corev1.ContainerPort{{ContainerPort: 5432}},
					Env: []corev1.EnvVar{
						{Name: "POSTGRES_USER", Value: pg.Spec.User},
						{Name: "POSTGRES_PASSWORD", Value: pg.Spec.Password},
						{Name: "PGDATA", Value: mountPath(pg) + "/pgdata"},
					},
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "data",
						MountPath: mountPath(pg),
					}},
				}},
				Volumes: []corev1.Volume{{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pg.Spec.Storage.PVCName,
						},
					},
				}},
			},
		}
		return nil
	})
	return wrap("Deployment", err)
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&Postgres{}).
		Complete(r)
}

func mountPath(pg *Postgres) string {
	if pg.Spec.Storage.MountPath != "" {
		return pg.Spec.Storage.MountPath
	}
	return "/var/lib/postgresql/data"
}

func wrap(resource string, err error) error {
	if err != nil {
		return fmt.Errorf("failed to reconcile %s: %w", resource, err)
	}
	return nil
}
