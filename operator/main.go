package main

import (
	"context"
	"net/http"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kdb.io/operator/metrics"
	"kdb.io/operator/mongo"
	"kdb.io/operator/postgres"
	"kdb.io/operator/redis"
)

var scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = postgres.AddToScheme(scheme)
	_ = redis.AddToScheme(scheme)
	_ = mongo.AddToScheme(scheme)
}

func main() {
	ctrl.SetLogger(zap.New())

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{Scheme: scheme})
	if err != nil {
		ctrl.Log.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := (&postgres.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create postgres controller")
		os.Exit(1)
	}

	if err := (&redis.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create redis controller")
		os.Exit(1)
	}

	if err := (&mongo.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create mongo controller")
		os.Exit(1)
	}

	// metrics endpoint
	metricsHandler := metrics.NewHTTPHandler(mgr.GetClient())
	mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		srv := &http.Server{Addr: ":9090", Handler: metricsHandler}
		go func() {
			<-ctx.Done()
			srv.Shutdown(context.Background())
		}()
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			return err
		}
		return nil
	}))

	ctrl.Log.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		ctrl.Log.Error(err, "problem running manager")
		os.Exit(1)
	}
}
