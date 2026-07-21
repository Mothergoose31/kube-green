package sleepinfo

import (
	"context"
	"testing"

	"github.com/kube-green/kube-green/api/v1alpha1"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

const (
	executionSleepInfoName = "execution-sleep"
	executionMockNow       = "2021-03-23T20:01:20.555Z"
	executionSleepTime     = "2021-03-23T20:05:59.000Z"
	// executionForcedOpTime is far (more than SleepDelta) from both the sleep
	// (20:05) and the wake up (20:20) cron ticks, to prove the execution
	// bypasses the schedule gate.
	executionForcedOpTime = "2021-03-23T20:07:00.000Z"
)

func TestSleepInfoExecution(t *testing.T) {
	const (
		sleepInfoName = executionSleepInfoName
		mockNow       = executionMockNow
		sleepTime     = executionSleepTime
	)
	testLogger := zap.New(zap.UseDevMode(true))
	testenv := testenvSetup(t)

	sleepNamespace := func(ctx context.Context, t *testing.T, c *envconf.Config) (ctrl.Request, originalResources) {
		t.Helper()
		reconciler := getSleepInfoReconciler(t, c, testLogger, mockNow)
		req, original := setupNamespaceWithResources(t, ctx, c, getDefaultSleepInfo(sleepInfoName, c.Namespace()), reconciler, getSetupOptions(t, ctx))

		sleepReconciler := getSleepInfoReconciler(t, c, testLogger, sleepTime)
		_, err := sleepReconciler.Reconcile(ctx, req)
		require.NoError(t, err)
		assertAllReplicasSetToZero(t, getDeploymentList(t, ctx, c))
		return req, original
	}

	forcedWakeUpWhileAsleep := features.New("forced wake up while asleep").
		WithSetup("namespace with resources asleep", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			_, original := sleepNamespace(ctx, t, c)
			return withAssertOperation(ctx, AssertOperation{originalResources: original})
		}).
		Assess("resources are restored at an arbitrary time", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			executionReconciler := getSleepInfoExecutionReconciler(t, c, testLogger)
			execution := createSleepInfoExecution(t, ctx, c, "wake-now", sleepInfoName, v1alpha1.ExecutionOpWake)

			result, err := executionReconciler.Reconcile(ctx, executionRequest(execution))
			require.NoError(t, err)
			require.Equal(t, ctrl.Result{}, result)

			deployments := getDeploymentList(t, ctx, c)
			for _, original := range assert.originalResources.deploymentList {
				actual := findDeployByName(deployments, original.GetName())
				require.NotNil(t, actual, original.GetName())
				require.Equal(t, deploymentReplicas(t, original), deploymentReplicas(t, *actual), original.GetName())
			}

			updated := getSleepInfoExecution(t, ctx, c, execution.GetName())
			require.Equal(t, v1alpha1.ExecutionPhaseSucceeded, updated.Status.Phase)
			require.NotNil(t, updated.Status.StartTime)
			require.NotNil(t, updated.Status.CompletionTime)
			require.Len(t, updated.GetOwnerReferences(), 1, "SleepInfo owner reference is set")
			require.Equal(t, sleepInfoName, updated.GetOwnerReferences()[0].Name)

			secret, err := executionReconciler.getSecret(ctx, getSecretName(sleepInfoName), c.Namespace())
			require.NoError(t, err)
			require.Equal(t, wakeUpOperation, string(secret.Data[lastOperationKey]))

			return ctx
		}).
		Feature()

	forcedWakeUpWhileAwake := features.New("forced wake up while not asleep is skipped").
		WithSetup("namespace with running resources", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			reconciler := getSleepInfoReconciler(t, c, testLogger, mockNow)
			_, original := setupNamespaceWithResources(t, ctx, c, getDefaultSleepInfo(sleepInfoName, c.Namespace()), reconciler, getSetupOptions(t, ctx))
			return withAssertOperation(ctx, AssertOperation{originalResources: original})
		}).
		Assess("execution is skipped and resources untouched", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			executionReconciler := getSleepInfoExecutionReconciler(t, c, testLogger)
			execution := createSleepInfoExecution(t, ctx, c, "wake-noop", sleepInfoName, v1alpha1.ExecutionOpWake)

			_, err := executionReconciler.Reconcile(ctx, executionRequest(execution))
			require.NoError(t, err)

			updated := getSleepInfoExecution(t, ctx, c, execution.GetName())
			require.Equal(t, v1alpha1.ExecutionPhaseSkipped, updated.Status.Phase)
			require.Equal(t, "resources are not asleep, nothing to wake up", updated.Status.Message)

			deployments := getDeploymentList(t, ctx, c)
			for _, original := range assert.originalResources.deploymentList {
				actual := findDeployByName(deployments, original.GetName())
				require.NotNil(t, actual, original.GetName())
				require.Equal(t, deploymentReplicas(t, original), deploymentReplicas(t, *actual), original.GetName())
			}

			return ctx
		}).
		Feature()

	forcedSleepWhileAsleep := features.New("forced sleep while already asleep is skipped").
		WithSetup("namespace with resources asleep", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			_, original := sleepNamespace(ctx, t, c)
			return withAssertOperation(ctx, AssertOperation{originalResources: original})
		}).
		Assess("execution is skipped and original state in secret is preserved", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			executionReconciler := getSleepInfoExecutionReconciler(t, c, testLogger)

			secretBefore, err := executionReconciler.getSecret(ctx, getSecretName(sleepInfoName), c.Namespace())
			require.NoError(t, err)
			require.NotEmpty(t, secretBefore.Data[originalJSONPatchDataKey], "original state is stored before the execution")

			execution := createSleepInfoExecution(t, ctx, c, "sleep-noop", sleepInfoName, v1alpha1.ExecutionOpSleep)
			_, err = executionReconciler.Reconcile(ctx, executionRequest(execution))
			require.NoError(t, err)

			updated := getSleepInfoExecution(t, ctx, c, execution.GetName())
			require.Equal(t, v1alpha1.ExecutionPhaseSkipped, updated.Status.Phase)
			require.Equal(t, "resources are already asleep", updated.Status.Message)

			secretAfter, err := executionReconciler.getSecret(ctx, getSecretName(sleepInfoName), c.Namespace())
			require.NoError(t, err)
			require.Equal(t, secretBefore.Data, secretAfter.Data, "secret is untouched")

			return ctx
		}).
		Feature()

	forcedSleepWhileAwake := features.New("forced sleep while awake").
		WithSetup("namespace with running resources", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			reconciler := getSleepInfoReconciler(t, c, testLogger, mockNow)
			_, original := setupNamespaceWithResources(t, ctx, c, getDefaultSleepInfo(sleepInfoName, c.Namespace()), reconciler, getSetupOptions(t, ctx))
			return withAssertOperation(ctx, AssertOperation{originalResources: original})
		}).
		Assess("resources are set to sleep at an arbitrary time", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			executionReconciler := getSleepInfoExecutionReconciler(t, c, testLogger)
			execution := createSleepInfoExecution(t, ctx, c, "sleep-now", sleepInfoName, v1alpha1.ExecutionOpSleep)

			_, err := executionReconciler.Reconcile(ctx, executionRequest(execution))
			require.NoError(t, err)

			assertAllReplicasSetToZero(t, getDeploymentList(t, ctx, c))

			updated := getSleepInfoExecution(t, ctx, c, execution.GetName())
			require.Equal(t, v1alpha1.ExecutionPhaseSucceeded, updated.Status.Phase)

			secret, err := executionReconciler.getSecret(ctx, getSecretName(sleepInfoName), c.Namespace())
			require.NoError(t, err)
			require.Equal(t, sleepOperation, string(secret.Data[lastOperationKey]))
			require.NotEmpty(t, secret.Data[originalJSONPatchDataKey], "original state is stored to restore later")

			return ctx
		}).
		Feature()

	sleepInfoNotFound := features.New("execution referencing a missing SleepInfo fails").
		Assess("execution fails with a clear message", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			executionReconciler := getSleepInfoExecutionReconciler(t, c, testLogger)
			execution := createSleepInfoExecution(t, ctx, c, "wake-missing", "not-existing-sleepinfo", v1alpha1.ExecutionOpWake)

			_, err := executionReconciler.Reconcile(ctx, executionRequest(execution))
			require.NoError(t, err)

			updated := getSleepInfoExecution(t, ctx, c, execution.GetName())
			require.Equal(t, v1alpha1.ExecutionPhaseFailed, updated.Status.Phase)
			require.Contains(t, updated.Status.Message, "not found")

			return ctx
		}).
		Feature()

	testenv.Test(t,
		forcedWakeUpWhileAsleep,
		forcedWakeUpWhileAwake,
		forcedSleepWhileAsleep,
		forcedSleepWhileAwake,
		sleepInfoNotFound,
	)
}

func getSleepInfoExecutionReconciler(t *testing.T, c *envconf.Config, logger logr.Logger) SleepInfoExecutionReconciler {
	t.Helper()

	sleepInfoReconciler := getSleepInfoReconciler(t, c, logger, executionForcedOpTime)

	k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()
	require.NoError(t, v1alpha1.AddToScheme(k8sClient.Scheme()))
	sleepInfoReconciler.Scheme = k8sClient.Scheme()

	return SleepInfoExecutionReconciler{
		SleepInfoReconciler: &sleepInfoReconciler,
	}
}

func createSleepInfoExecution(t *testing.T, ctx context.Context, c *envconf.Config, name, sleepInfoName string, operation v1alpha1.SleepInfoExecutionOperation) *v1alpha1.SleepInfoExecution {
	t.Helper()

	execution := &v1alpha1.SleepInfoExecution{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SleepInfoExecution",
			APIVersion: "kube-green.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.Namespace(),
		},
		Spec: v1alpha1.SleepInfoExecutionSpec{
			SleepInfoName: sleepInfoName,
			Operation:     operation,
		},
	}

	k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()
	require.NoError(t, v1alpha1.AddToScheme(k8sClient.Scheme()))
	require.NoError(t, k8sClient.Create(ctx, execution))

	return execution
}

func getSleepInfoExecution(t *testing.T, ctx context.Context, c *envconf.Config, name string) *v1alpha1.SleepInfoExecution {
	t.Helper()

	k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()
	execution := &v1alpha1.SleepInfoExecution{}
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: c.Namespace()}, execution))
	return execution
}

func executionRequest(execution *v1alpha1.SleepInfoExecution) ctrl.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      execution.GetName(),
			Namespace: execution.GetNamespace(),
		},
	}
}
