package mongo

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
	mongo := &Mongo{}
	if err := r.Get(ctx, req.NamespacedName, mongo); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get Mongo: %w", err)
	}

	image := mongo.Spec.Image
	if image == "" {
		image = "mongo:8.2"
	}

	if err := r.reconcilePVC(ctx, mongo); err != nil {
		r.setPhase(ctx, mongo, "Error")
		return ctrl.Result{}, err
	}
	if err := r.reconcileIngressRouteTCP(ctx, mongo); err != nil {
		r.setPhase(ctx, mongo, "Error")
		return ctrl.Result{}, err
	}
	if err := r.reconcileService(ctx, mongo); err != nil {
		r.setPhase(ctx, mongo, "Error")
		return ctrl.Result{}, err
	}
	if err := r.reconcileDeployment(ctx, mongo, image); err != nil {
		r.setPhase(ctx, mongo, "Error")
		return ctrl.Result{}, err
	}
	r.setPhase(ctx, mongo, "Ready")
	return ctrl.Result{}, nil
}

func (r *Reconciler) setPhase(ctx context.Context, mongo *Mongo, phase string) {
	mongo.Status.Phase = phase
	if err := r.Status().Update(ctx, mongo); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "failed to update status", "phase", phase)
	}
}

func (r *Reconciler) reconcilePVC(ctx context.Context, mongo *Mongo) error {
	storageClass := mongo.Spec.Storage.StorageClass
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: mongo.Spec.Storage.PVCName, Namespace: mongo.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		if err := controllerutil.SetControllerReference(mongo, pvc, r.Scheme); err != nil {
			return err
		}
		if pvc.CreationTimestamp.IsZero() {
			modes := make([]corev1.PersistentVolumeAccessMode, len(mongo.Spec.Storage.AccessModes))
			for i, m := range mongo.Spec.Storage.AccessModes {
				modes[i] = corev1.PersistentVolumeAccessMode(m)
			}
			pvc.Spec.AccessModes = modes
			pvc.Spec.StorageClassName = &storageClass
			pvc.Spec.Resources = corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(mongo.Spec.Storage.Size),
				},
			}
		}
		return nil
	})
	return wrap("PVC", err)
}

func mountPath(mongo *Mongo) string {
	if mongo.Spec.Storage.MountPath != "" {
		return mongo.Spec.Storage.MountPath
	}
	return "/data/db"
}

func (r *Reconciler) reconcileIngressRouteTCP(ctx context.Context, mongo *Mongo) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "traefik.io", Version: "v1alpha1", Kind: "IngressRouteTCP"})
	obj.SetName(mongo.Name)
	obj.SetNamespace(mongo.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
		if err := controllerutil.SetControllerReference(mongo, obj, r.Scheme); err != nil {
			return err
		}
		obj.Object["spec"] = map[string]interface{}{
			"entryPoints": []interface{}{"tcp"},
			"routes": []interface{}{
				map[string]interface{}{
					"match": fmt.Sprintf("HostSNI(`%s`)", mongo.Spec.Domain),
					"services": []interface{}{
						map[string]interface{}{"name": mongo.Name, "port": int64(27017)},
					},
				},
			},
			"tls": map[string]interface{}{
				"secretName": "tls-cert",
			},
		}
		return nil
	})
	return wrap("IngressRouteTCP", err)
}

func (r *Reconciler) reconcileService(ctx context.Context, mongo *Mongo) error {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: mongo.Name, Namespace: mongo.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if err := controllerutil.SetControllerReference(mongo, svc, r.Scheme); err != nil {
			return err
		}
		svc.Spec.Selector = map[string]string{"app": mongo.Name}
		svc.Spec.Ports = []corev1.ServicePort{
			{Name: "mongo", Port: 27017, TargetPort: intstr.FromInt32(27017)},
		}
		return nil
	})
	return wrap("Service", err)
}

func (r *Reconciler) reconcileDeployment(ctx context.Context, mongo *Mongo, image string) error {
	replicas := int32(1)
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: mongo.Name, Namespace: mongo.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		if err := controllerutil.SetControllerReference(mongo, dep, r.Scheme); err != nil {
			return err
		}
		labels := map[string]string{"app": mongo.Name}
		if dep.CreationTimestamp.IsZero() {
			dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		}
		dep.Spec.Replicas = &replicas
		dep.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name:  "mongo",
					Image: image,
					Ports: []corev1.ContainerPort{{ContainerPort: 27017}},
					Env: []corev1.EnvVar{
						{Name: "MONGO_INITDB_ROOT_USERNAME", Value: mongo.Spec.User},
						{Name: "MONGO_INITDB_ROOT_PASSWORD", Value: mongo.Spec.Password},
					},
					VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: mountPath(mongo)}},
				}},
				Volumes: []corev1.Volume{{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: mongo.Spec.Storage.PVCName,
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
		For(&Mongo{}).
		Complete(r)
}

func wrap(resource string, err error) error {
	if err != nil {
		return fmt.Errorf("failed to reconcile %s: %w", resource, err)
	}
	return nil
}
