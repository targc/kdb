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

	"kdb.io/operator/portalloc"
)

type Reconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	PortAlloc *portalloc.Allocator
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pg := &Postgres{}
	if err := r.Get(ctx, req.NamespacedName, pg); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get Postgres: %w", err)
	}

	// Register/run the port-release finalizer. On deletion this frees the
	// allocation and stops before re-allocating below.
	if stop, err := r.PortAlloc.HandleFinalizer(ctx, pg); err != nil {
		return ctrl.Result{}, err
	} else if stop {
		return ctrl.Result{}, nil
	}

	// Allocate port on LB node (idempotent — returns existing if already assigned)
	alloc, err := r.PortAlloc.Allocate(ctx, req.NamespacedName.String())
	if err != nil {
		r.setPhase(ctx, pg, "Error", err.Error())
		return ctrl.Result{}, fmt.Errorf("failed to allocate port: %w", err)
	}
	if pg.Status.Port == 0 {
		pg.Status.Port = alloc.Port
		pg.Status.Host = alloc.Host
	}

	image := pg.Spec.Image
	if image == "" {
		image = "postgres:16"
	}

	for _, fn := range []func() error{
		func() error { return r.reconcilePVC(ctx, pg) },
		func() error { return r.reconcileIngressRouteTCP(ctx, pg, alloc) },
		func() error { return r.reconcileService(ctx, pg) },
		func() error { return r.reconcileDeployment(ctx, pg, image) },
	} {
		if err := fn(); err != nil {
			if errors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			r.setPhase(ctx, pg, "Error", err.Error())
			return ctrl.Result{}, err
		}
	}
	r.setPhase(ctx, pg, "Running")
	return ctrl.Result{}, nil
}

func (r *Reconciler) setPhase(ctx context.Context, pg *Postgres, phase string, msgs ...string) {
	pg.Status.Phase = phase
	if len(msgs) > 0 {
		pg.Status.Message = msgs[0]
	} else {
		pg.Status.Message = ""
	}
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
		}
		pvc.Spec.Resources = corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(pg.Spec.Storage.Size),
			},
		}
		return nil
	})
	return wrap("PVC", err)
}

func (r *Reconciler) reconcileIngressRouteTCP(ctx context.Context, pg *Postgres, alloc *portalloc.Allocation) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "traefik.io", Version: "v1alpha1", Kind: "IngressRouteTCP"})
	obj.SetName(pg.Name)
	obj.SetNamespace(pg.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
		if err := controllerutil.SetControllerReference(pg, obj, r.Scheme); err != nil {
			return err
		}
		obj.SetLabels(map[string]string{"kdb.io/lb-node": alloc.Node})
		obj.Object["spec"] = map[string]interface{}{
			"entryPoints": []interface{}{fmt.Sprintf("tcp-%d", alloc.Port)},
			"routes": []interface{}{
				map[string]interface{}{
					"match": "HostSNI(`*`)",
					"services": []interface{}{
						map[string]interface{}{"name": pg.Name, "port": int64(5432)},
					},
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
		env := []corev1.EnvVar{
			{Name: "POSTGRES_USER", Value: pg.Spec.User},
			{Name: "POSTGRES_PASSWORD", Value: pg.Spec.Password},
			{Name: "PGDATA", Value: mountPath(pg) + "/pgdata"},
		}
		if pg.Spec.Database != "" {
			env = append(env, corev1.EnvVar{Name: "POSTGRES_DB", Value: pg.Spec.Database})
		}
		dep.Spec.Replicas = &replicas
		// Recreate, not the default RollingUpdate: replicas=1 on a ReadWriteOnce PVC means a
		// surge-first rollout tries to attach the volume to a new pod before the old one
		// releases it — Multi-Attach error, pod stuck Pending. Recreate fully terminates the
		// old pod (and detaches the PVC) before creating the new one, so that race can't happen.
		dep.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
		dep.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec: corev1.PodSpec{
				// postgres:16 (Debian-based) conventionally runs as uid/gid 999 — NOT verified
				// against a live pod, unlike redis's confirmed 999 (see redis/controller.go).
				// fsGroup makes kubelet chown the PVC mount to that group so it's writable
				// regardless of whether the underlying volume starts out root-owned (e.g. a
				// freshly formatted Longhorn disk). Confirm with `kubectl exec <pod> -- id postgres`
				// before relying on this in production.
				SecurityContext: &corev1.PodSecurityContext{FSGroup: int64Ptr(999)},
				Containers: []corev1.Container{{
					Name:      "postgres",
					Image:     image,
					Ports:     []corev1.ContainerPort{{ContainerPort: 5432}},
					Env:       env,
					Resources: resourceRequirements(pg.Spec.Resources),
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
