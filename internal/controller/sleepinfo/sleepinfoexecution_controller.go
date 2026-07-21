/*
Copyright 2021.
*/

package sleepinfo

import (
	"context"
	"fmt"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/jsonpatch"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/resource"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// SleepInfoExecutionReconciler reconciles a SleepInfoExecution object.
// It performs an on-demand sleep or wake up of the resources managed by the
// referenced SleepInfo, bypassing the cron schedule gate used by the
// SleepInfoReconciler.
type SleepInfoExecutionReconciler struct {
	*SleepInfoReconciler
}

func (r *SleepInfoExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("sleepinfoexecution", req.NamespacedName)

	execution := &kubegreenv1alpha1.SleepInfoExecution{}
	if err := r.Get(ctx, req.NamespacedName, execution); err != nil {
		log.V(8).Info("unable to fetch sleepInfoExecution", "err", err.Error())
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if execution.IsProcessed() {
		log.V(8).Info("execution already processed", "phase", execution.Status.Phase)
		return ctrl.Result{}, nil
	}

	now := r.Now()

	sleepInfo := &kubegreenv1alpha1.SleepInfo{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: execution.Spec.SleepInfoName}, sleepInfo); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, r.updateExecutionStatus(ctx, execution, now, kubegreenv1alpha1.ExecutionPhaseFailed, fmt.Sprintf("SleepInfo %q not found in namespace %q", execution.Spec.SleepInfoName, req.Namespace))
		}
		return ctrl.Result{}, err
	}

	// The SleepInfo owns the execution, so deleting the SleepInfo garbage
	// collects its execution history.
	if err := r.setOwnerReference(ctx, execution, sleepInfo); err != nil {
		log.Error(err, "fails to set owner reference")
		return ctrl.Result{}, err
	}

	secretName := getSecretName(sleepInfo.Name)
	secret, err := r.getSecret(ctx, secretName, req.Namespace)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	// The operation-type key of the secret is the source of truth about the
	// namespace state: it is SLEEP only while the resources are asleep.
	isAsleep := secret != nil && string(secret.Data[lastOperationKey]) == sleepOperation

	// Guards: a wake up without a previous sleep has no original state to
	// restore, and a sleep while already asleep would overwrite the stored
	// original state with the scaled-down one, losing the restore data.
	switch {
	case execution.Spec.Operation == kubegreenv1alpha1.ExecutionOpWake && !isAsleep:
		return ctrl.Result{}, r.updateExecutionStatus(ctx, execution, now, kubegreenv1alpha1.ExecutionPhaseSkipped, "resources are not asleep, nothing to wake up")
	case execution.Spec.Operation == kubegreenv1alpha1.ExecutionOpSleep && isAsleep:
		return ctrl.Result{}, r.updateExecutionStatus(ctx, execution, now, kubegreenv1alpha1.ExecutionPhaseSkipped, "resources are already asleep")
	}

	sleepInfoData, err := getSleepInfoData(secret, sleepInfo)
	if err != nil {
		log.Error(err, "unable to get secret data")
		return ctrl.Result{}, err
	}
	sleepInfoData, err = applyExecutionOverride(sleepInfoData, execution.Spec.Operation, sleepInfo)
	if err != nil {
		// Configuration errors are permanent, e.g. a WAKE_UP requested for a
		// SleepInfo without wakeUpAt: do not retry.
		return ctrl.Result{}, r.updateExecutionStatus(ctx, execution, now, kubegreenv1alpha1.ExecutionPhaseFailed, err.Error())
	}

	resources, err := jsonpatch.NewResources(ctx, resource.ResourceClient{
		Client:           r.Client,
		SleepInfo:        sleepInfo,
		Log:              log,
		FieldManagerName: r.ManagerName,
	}, req.Namespace, sleepInfoData.OriginalGenericResourceInfo)
	if err != nil {
		log.Error(err, "fails to get resources")
		return ctrl.Result{}, err
	}

	if !resources.HasResource() {
		return ctrl.Result{}, r.updateExecutionStatus(ctx, execution, now, kubegreenv1alpha1.ExecutionPhaseSkipped, "no resources to handle in namespace")
	}

	switch {
	case sleepInfoData.IsSleepOperation():
		err = resources.Sleep(ctx)
	case sleepInfoData.IsWakeUpOperation():
		err = resources.WakeUp(ctx)
	}
	if err != nil {
		// Transient failure: leave the execution non terminal so it is retried
		// with backoff.
		log.Error(err, "fails to handle operation", "operation", execution.Spec.Operation)
		return ctrl.Result{}, err
	}

	if err := r.upsertSecret(ctx, log, now, secretName, req.Namespace, sleepInfo, secret, sleepInfoData, resources); err != nil {
		log.Error(err, "fails to update secret", "secret", secretName)
		return ctrl.Result{}, err
	}
	if err := r.handleSleepInfoStatus(ctx, now, sleepInfo, sleepInfoData.CurrentOperationType, resources); err != nil {
		log.Error(err, "unable to update sleepInfo status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, r.updateExecutionStatus(ctx, execution, now, kubegreenv1alpha1.ExecutionPhaseSucceeded, fmt.Sprintf("operation %s executed", execution.Spec.Operation))
}

// SetupWithManager sets up the controller with the Manager.
func (r *SleepInfoExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Clock == nil {
		r.Clock = realClock{}
	}

	// The spec is immutable, so with the generation changed predicate every
	// execution is processed exactly once at creation: owner reference and
	// status updates do not retrigger the reconcile.
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubegreenv1alpha1.SleepInfoExecution{}).
		Named("kubegreen-sleepinfoexecution").
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

func (r *SleepInfoExecutionReconciler) setOwnerReference(ctx context.Context, execution *kubegreenv1alpha1.SleepInfoExecution, sleepInfo *kubegreenv1alpha1.SleepInfo) error {
	ownerReferencesCount := len(execution.GetOwnerReferences())
	if err := controllerutil.SetOwnerReference(sleepInfo, execution, r.Scheme); err != nil {
		return err
	}
	if len(execution.GetOwnerReferences()) == ownerReferencesCount {
		return nil
	}
	return r.Update(ctx, execution)
}

func (r *SleepInfoExecutionReconciler) updateExecutionStatus(ctx context.Context, execution *kubegreenv1alpha1.SleepInfoExecution, now time.Time, phase kubegreenv1alpha1.SleepInfoExecutionPhase, message string) error {
	metaNow := metav1.NewTime(now)
	if execution.Status.StartTime == nil {
		execution.Status.StartTime = &metaNow
	}
	execution.Status.CompletionTime = &metaNow
	execution.Status.Phase = phase
	execution.Status.Message = message
	return r.Status().Update(ctx, execution)
}
