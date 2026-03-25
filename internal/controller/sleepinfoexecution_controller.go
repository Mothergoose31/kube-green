/*
Copyright 2024.
*/

package sleepinfo

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	kubegreencomv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
)

// SleepinfoExecutionReconciler reconciles a SleepinfoExecution object
type SleepinfoExecutionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kube-green.com,resources=sleepinfoexecutions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kube-green.com,resources=sleepinfoexecutions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kube-green.com,resources=sleepinfoexecutions/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SleepinfoExecution object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
func (r *SleepinfoExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SleepinfoExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubegreencomv1alpha1.SleepinfoExecution{}).
		Named("sleepinfoexecution").
		Complete(r)
}
