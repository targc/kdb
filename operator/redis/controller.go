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

	"kdb.io/operator/portalloc"
)

type Reconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	PortAlloc *portalloc.Allocator
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	redis := &Redis{}
	if err := r.Get(ctx, req.NamespacedName, redis); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get Redis: %w", err)
	}

	// Register/run the port-release finalizer. On deletion this frees the
	// allocation and stops before re-allocating below.
	if stop, err := r.PortAlloc.HandleFinalizer(ctx, redis); err != nil {
		return ctrl.Result{}, err
	} else if stop {
		return ctrl.Result{}, nil
	}

	// Allocate port on LB node (idempotent — returns existing if already assigned)
	alloc, err := r.PortAlloc.Allocate(ctx, req.NamespacedName.String())
	if err != nil {
		r.setPhase(ctx, redis, "Error", err.Error())
		return ctrl.Result{}, fmt.Errorf("failed to allocate port: %w", err)
	}
	if redis.Status.Port == 0 {
		redis.Status.Port = alloc.Port
		redis.Status.Host = alloc.Host
	}

	image := redis.Spec.Image
	if image == "" {
		image = "redis:8"
	}

	for _, fn := range []func() error{
		func() error { return r.reconcilePVC(ctx, redis) },
		func() error { return r.reconcileIngressRouteTCP(ctx, redis, alloc) },
		func() error { return r.reconcileService(ctx, redis) },
		func() error { return r.reconcileDeployment(ctx, redis, image) },
	} {
		if err := fn(); err != nil {
			if errors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			r.setPhase(ctx, redis, "Error", err.Error())
			return ctrl.Result{}, err
		}
	}
	r.setPhase(ctx, redis, "Ready")
	return ctrl.Result{}, nil
}

func (r *Reconciler) setPhase(ctx context.Context, redis *Redis, phase string, msgs ...string) {
	redis.Status.Phase = phase
	if len(msgs) > 0 {
		redis.Status.Message = msgs[0]
	} else {
		redis.Status.Message = ""
	}
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
		}
		pvc.Spec.Resources = corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(redis.Spec.Storage.Size),
			},
		}
		return nil
	})
	return wrap("PVC", err)
}

func (r *Reconciler) reconcileIngressRouteTCP(ctx context.Context, redis *Redis, alloc *portalloc.Allocation) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "traefik.io", Version: "v1alpha1", Kind: "IngressRouteTCP"})
	obj.SetName(redis.Name)
	obj.SetNamespace(redis.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
		if err := controllerutil.SetControllerReference(redis, obj, r.Scheme); err != nil {
			return err
		}
		obj.SetLabels(map[string]string{"kdb.io/lb-node": alloc.Node})
		obj.Object["spec"] = map[string]interface{}{
			"entryPoints": []interface{}{fmt.Sprintf("tcp-%d", alloc.Port)},
			"routes": []interface{}{
				map[string]interface{}{
					"match": "HostSNI(`*`)",
					"services": []interface{}{
						map[string]interface{}{"name": redis.Name, "port": int64(6379)},
					},
				},
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
			Name:      "redis",
			Image:     image,
			Ports:     []corev1.ContainerPort{{ContainerPort: 6379}},
			Resources: resourceRequirements(redis.Spec.Resources),
		}
		container.Args = []string{"--requirepass", redis.Spec.Password}

		container.VolumeMounts = []corev1.VolumeMount{{Name: "data", MountPath: mountPath(redis)}}
		dep.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec: corev1.PodSpec{
				// redis:8 runs as uid/gid 999 (confirmed via `id redis` in-cluster) — fsGroup
				// makes kubelet chown the PVC mount to that group, so it's writable regardless
				// of whether the underlying volume (e.g. a freshly formatted Longhorn disk)
				// starts out root-owned.
				SecurityContext: &corev1.PodSecurityContext{FSGroup: int64Ptr(999)},
				Containers:      []corev1.Container{container},
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

func mountPath(redis *Redis) string {
	if redis.Spec.Storage.MountPath != "" {
		return redis.Spec.Storage.MountPath
	}
	return "/data"
}

func int64Ptr(v int64) *int64 { return &v }

func wrap(name string, err error) error {
	if err != nil {
		return fmt.Errorf("failed to reconcile %s: %w", name, err)
	}
	return nil
}

// resourceRequirements maps the spec's cpu/memory strings to container requests/limits. Empty or
// unparseable values are skipped (defensive — the backend and CRD already validate), so unset fields
// simply fall back to the namespace/operator defaults.
func resourceRequirements(rs ResourcesSpec) corev1.ResourceRequirements {
	var requests, limits corev1.ResourceList
	add := func(list *corev1.ResourceList, name corev1.ResourceName, qty string) {
		if qty == "" {
			return
		}
		q, err := resource.ParseQuantity(qty)
		if err != nil {
			return
		}
		if *list == nil {
			*list = corev1.ResourceList{}
		}
		(*list)[name] = q
	}
	add(&requests, corev1.ResourceCPU, rs.Requests.CPU)
	add(&requests, corev1.ResourceMemory, rs.Requests.Memory)
	add(&limits, corev1.ResourceCPU, rs.Limits.CPU)
	add(&limits, corev1.ResourceMemory, rs.Limits.Memory)
	return corev1.ResourceRequirements{Requests: requests, Limits: limits}
}
