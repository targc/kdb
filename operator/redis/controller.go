package redis

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
	redis := &Redis{}
	if err := r.Get(ctx, req.NamespacedName, redis); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get Redis: %w", err)
	}

	image := redis.Spec.Image
	if image == "" {
		image = "redis:8"
	}

	for _, fn := range []func() error{
		func() error { return r.reconcilePVC(ctx, redis) },
		func() error { return r.reconcileIngressRouteTCP(ctx, redis) },
		func() error { return r.reconcileService(ctx, redis) },
		func() error { return r.reconcileDeployment(ctx, redis, image) },
	} {
		if err := fn(); err != nil {
			if errors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			r.setPhase(ctx, redis, "Error")
			return ctrl.Result{}, err
		}
	}
	r.setPhase(ctx, redis, "Ready")
	return ctrl.Result{}, nil
}

func (r *Reconciler) setPhase(ctx context.Context, redis *Redis, phase string) {
	redis.Status.Phase = phase
	if err := r.Status().Update(ctx, redis); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "failed to update status", "phase", phase)
	}
}

func (r *Reconciler) reconcilePVC(ctx context.Context, redis *Redis) error {
	storageClass := redis.Spec.Storage.StorageClass
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: redis.Spec.Storage.PVCName, Namespace: redis.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		if err := controllerutil.SetControllerReference(redis, pvc, r.Scheme); err != nil {
			return err
		}
		if pvc.CreationTimestamp.IsZero() {
			modes := make([]corev1.PersistentVolumeAccessMode, len(redis.Spec.Storage.AccessModes))
			for i, m := range redis.Spec.Storage.AccessModes {
				modes[i] = corev1.PersistentVolumeAccessMode(m)
			}
			pvc.Spec.AccessModes = modes
			pvc.Spec.StorageClassName = &storageClass
			pvc.Spec.Resources = corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(redis.Spec.Storage.Size),
				},
			}
		}
		return nil
	})
	return wrap("PVC", err)
}

func mountPath(redis *Redis) string {
	if redis.Spec.Storage.MountPath != "" {
		return redis.Spec.Storage.MountPath
	}
	return "/data"
}

func (r *Reconciler) reconcileIngressRouteTCP(ctx context.Context, redis *Redis) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "traefik.io", Version: "v1alpha1", Kind: "IngressRouteTCP"})
	obj.SetName(redis.Name)
	obj.SetNamespace(redis.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
		if err := controllerutil.SetControllerReference(redis, obj, r.Scheme); err != nil {
			return err
		}
		obj.Object["spec"] = map[string]interface{}{
			"entryPoints": []interface{}{"tcp"},
			"routes": []interface{}{
				map[string]interface{}{
					"match": fmt.Sprintf("HostSNI(`%s`)", redis.Spec.Domain),
					"services": []interface{}{
						map[string]interface{}{"name": redis.Name, "port": int64(6379)},
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

func (r *Reconciler) reconcileService(ctx context.Context, redis *Redis) error {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: redis.Name, Namespace: redis.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if err := controllerutil.SetControllerReference(redis, svc, r.Scheme); err != nil {
			return err
		}
		svc.Spec.Selector = map[string]string{"app": redis.Name}
		svc.Spec.Ports = []corev1.ServicePort{
			{Name: "redis", Port: 6379, TargetPort: intstr.FromInt32(6379)},
		}
		return nil
	})
	return wrap("Service", err)
}

func (r *Reconciler) reconcileDeployment(ctx context.Context, redis *Redis, image string) error {
	replicas := int32(1)
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: redis.Name, Namespace: redis.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		if err := controllerutil.SetControllerReference(redis, dep, r.Scheme); err != nil {
			return err
		}
		labels := map[string]string{"app": redis.Name}
		if dep.CreationTimestamp.IsZero() {
			dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		}
		dep.Spec.Replicas = &replicas

		container := corev1.Container{
			Name:  "redis",
			Image: image,
			Ports: []corev1.ContainerPort{{ContainerPort: 6379}},
		}
		if redis.Spec.Password != "" {
			container.Args = []string{"--requirepass", redis.Spec.Password}
		}

		container.VolumeMounts = []corev1.VolumeMount{{Name: "data", MountPath: mountPath(redis)}}
		dep.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{container},
				Volumes: []corev1.Volume{{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: redis.Spec.Storage.PVCName,
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
		For(&Redis{}).
		Complete(r)
}

func wrap(resource string, err error) error {
	if err != nil {
		return fmt.Errorf("failed to reconcile %s: %w", resource, err)
	}
	return nil
}
