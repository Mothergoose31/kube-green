/*
Copyright 2024.
*/

package sleepinfo

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	kubegreencomv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/resource"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// SleepinfoExecutionReconciler reconciles a SleepinfoExecution object
type SleepinfoExecutionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Clock
	Log         logr.Logger
	ManagerName string
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
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: req.Namespace,
		Name:      exec.Spec.SleepInfoRef,
	}, sleepInfo); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, r.failExecution(ctx, exec, gen,
				fmt.Sprintf("SleepInfo %q not found", exec.Spec.SleepInfoRef))
		}
		log.Error(err, "get SleepInfo")
		return ctrl.Result{}, err
	}

	if err := controllerutil.SetControllerReference(sleepInfo, exec, r.Scheme); err != nil {
		log.Error(err, "set controller reference")
		return ctrl.Result{}, err
	}
	if err := r.Update(ctx, exec); err != nil {
		log.Error(err, "update owner references on SleepInfoExecution")
		return ctrl.Result{}, err
	}

	secretName := getSecretName(sleepInfo.Name)
	// TODO  load state

	return ctrl.Result{}, r.patchExecutionStatus(ctx, exec, gen, kubegreencomv1alpha1.PhaseSucceeded, "")
}

func (r *SleepinfoExecutionReconciler) executeOperation(ctx context.Context, log logr.Logger, data SleepInfoData, resources resource.Resource) error {
	switch {
	case data.IsSleepOperation():
		if err := resources.Sleep(ctx); err != nil {
			log.Error(err, "sleep failed")
			return fmt.Errorf("sleep failed: %w", err)
		}
	case data.IsWakeUpOperation():
		if err := resources.WakeUp(ctx); err != nil {
			log.Error(err, "wake up failed")
			return fmt.Errorf("wake up failed: %w", err)
		}
	default:
		return fmt.Errorf("unsupported operation %q", data.CurrentOperationType)
	}
	return nil
}

func (r *SleepinfoExecutionReconciler) patchExecutionStatus(
	ctx context.Context,
	exec *kubegreencomv1alpha1.SleepinfoExecution,
	gen int64,
	phase kubegreencomv1alpha1.SleepInfoExecutionPhase,
	msg string,
) error {
	patch := client.MergeFrom(exec.DeepCopy())
	exec.Status.Phase = phase
	exec.Status.ObservedGeneration = gen
	exec.Status.Message = msg
	return r.Status().Patch(ctx, exec, patch)
}

func (r *SleepinfoExecutionReconciler) failExecution(
	ctx context.Context,
	exec *kubegreencomv1alpha1.SleepinfoExecution,
	gen int64,
	msg string,
) error {
	return r.patchExecutionStatus(ctx, exec, gen, kubegreencomv1alpha1.PhaseFailed, msg)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SleepinfoExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Clock == nil {
		r.Clock = realClock{}
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubegreencomv1alpha1.SleepinfoExecution{}).
		Named("sleepinfoexecution").
		Complete(r)
}
