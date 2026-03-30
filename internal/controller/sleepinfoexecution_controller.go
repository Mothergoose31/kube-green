/*
Copyright 2024.
*/

package sleepinfo

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	kubegreencomv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
)

// SleepinfoExecutionReconciler reconciles a SleepinfoExecution object
type SleepinfoExecutionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Clock
	Log                     *logr.Logger
	ManagerName             string
	MaxConcurrentReconciles int
}

// +kubebuilder:rbac:groups=kube-green.com,resources=sleepinfoexecutions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kube-green.com,resources=sleepinfoexecutions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kube-green.com,resources=sleepinfoexecutions/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// the SleepinfoExecution object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
func (r *SleepinfoExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("sleepinfoexecution", req.NamespacedName)

	exec := &kubegreencomv1alpha1.SleepinfoExecution{}
	if err := r.Get(ctx, req.NamespacedName, exec); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	gen := exec.GetGeneration()
	if exec.Status.Phase == kubegreencomv1alpha1.PhaseSucceeded &&
		exec.Status.ObservedGeneration == gen {
		return ctrl.Result{}, nil
	}
	sleepInfo := &kubegreencomv1alpha1.SleepInfo{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: exec.Spec.SleepInfoRef}, sleepInfo); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, r.failExecution(ctx, exec, gen, fmt.Sprintf("SleepInfo %q not found", exec.Spec.SleepInfoRef))
		}
		log.Error(err, "get SleepInfo")
		return ctrl.Result{}, err
	}
	if err := controllerutil.SetControllerReference(sleepInfo, exec, *r.Scheme, controllerutil.WithBlockOwnerDeletion(false)); err != nil {
		log.Error(err, "set controller reference")
		return ctrl.Result{}, err

	}
	if err := r.Update(ctx, exec); err != nil {
		log.Error(err, "update owner references on SleepInfoExecution")
		return ctrl.Result{}, err
	}
	secretName := getSecretName(sleepInfo.Name)
	secret, err := fetchSecret(ctx, r.Client, log, secretName, req.NamespacedName)
	if client.IgnoreNotFound(err) != nil {
		log.Error(err, "get secret")
		return ctrl.Result{}, err
	}
	sleepInfoData, err := sleepInfoData(secret, sleepInfo)
	if err != nil {
		return ctrl.Result{}, r.failExecution(ctx, exec, gen, fmt.Sprintf("invalid sleepinfo secret data: %v", err))
	}
	op := string(exec.Spec.Operation)
	data, err := ApplyExecutionOverride(sleepInfoData, op, sleepInfo)
	if err != nil {
		return ctrl.Result{}, r.failExecution(ctx, exec, gen, err.Error())
	}

	if err := patchExecutionStatus(ctx, exec, gen, kubegreencomv1alpha1.PhaseRunning, "", metav1.ConditionUnknown); err != nil {
		return ctrl.Result{}, err
	}
	now := r.Now()
	// TODO create runner

	return ctrl.Result{}, nil
}

func (r *SleepinfoExecutionReconciler) failExecution(ctx context.Context, exec *kubegreencomv1alpha1.SleepinfoExecution, gen int64, msg string) error {
	return r.patch
}

// SetupWithManager sets up the controller with the Manager.
func (r *SleepinfoExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubegreencomv1alpha1.SleepinfoExecution{}).
		Named("sleepinfoexecution").
		Complete(r)
}
