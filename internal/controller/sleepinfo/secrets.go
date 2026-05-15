package sleepinfo

import (
	"context"
	"fmt"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/resource"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"github.com/go-logr/logr"
)

type StateStore struct {
	c           client.Client
	managerName string
}

func NewStateStore(c client.Client, managerName string) *StateStore {
	return &StateStore{c: c, managerName: managerName}
}

func getSecretName(name string) string {
	return fmt.Sprintf("sleepinfo-%s", name)
}

// get  secret and add it into SleepInfoData ,return nil if no secret is where
func (s *StateStore) Load(ctx context.Context, sleepInfo *kubegreenv1alpha1.SleepInfo) (*v1.Secret, SleepInfoData, error) {
	secretName := getSecretName(sleepInfo.Name)
	secret := &v1.Secret{}
	if err := s.c.Get(ctx, client.ObjectKey{
		Namespace: sleepInfo.Namespace,
		Name:      secretName,
	}, secret); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, SleepInfoData{}, fmt.Errorf("getting tracking secret %s:%w", secretName, err)
		}
		secret = nil
	}
	data, err := getSleepInfoData(secret, sleepInfo)
	if err != nil {
		return nil, SleepInfoData{}, fmt.Errorf("phrase tracking secret %w", err)
	}
	return secret, data, nil
}

func (s *StateStore) Save(
	ctx context.Context,
	sleepInfo *kubegreenv1alpha1.SleepInfo,
	existing *v1.Secret,
	now time.Time,
	data SleepInfoData,
	resources resource.Resource,
) error {
	desired, err := s.buildSecret(sleepInfo, now, data, resources)
	if err != nil {
		return fmt.Errorf("build tracking secret: %w", err)
	}
	if existing == nil {
		return s.c.Create(ctx, desired)
	}

	desired.ResourceVersion = existing.ResourceVersion
	return s.c.Update(ctx, desired)
}

func (s *StateStore) buildSecret(
	sleepInfo *kubegreenv1alpha1.SleepInfo,
	now time.Time,
	data SleepInfoData,
	resources resource.Resource,
) (*v1.Secret, error) {
	secret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      getSecretName(sleepInfo.Name),
			Namespace: sleepInfo.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": s.managerName,
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: kubegreenv1alpha1.GroupVersion.String(),
				Kind:       "SleepInfo",
				Name:       sleepInfo.Name,
				UID:        sleepInfo.UID,
			}},
		},
		Data: map[string][]byte{
			lastScheduleKey: []byte(now.Format(time.RFC3339)),
		},
	}

	if resources.HasResource() {
		secret.Data[lastOperationKey] = []byte(data.CurrentOperationType)
	}

	if resources.HasResource() && data.IsSleepOperation() {

		orig, err := resources.GetOriginalInfoToSave()
		if err != nil {
			return nil, fmt.Errorf("get original resource info: %w", err)
		}
		secret.Data[originalJSONPatchDataKey] = orig
	}
	return secret, nil
}

func (r *SleepInfoReconciler) getSecret(ctx context.Context, name, namespace string) (*v1.Secret, error) {
	secret := &v1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, secret); err != nil {
		return nil, err
	}
	return secret, nil
}

func (r *SleepInfoReconciler) upsertSecret(
	ctx context.Context,
	_ logr.Logger,
	now time.Time,
	secretName, namespace string,
	sleepInfo *kubegreenv1alpha1.SleepInfo,
	existing *v1.Secret,
	data SleepInfoData,
	resources resource.Resource,
) error {
	secret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": r.ManagerName,
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: kubegreenv1alpha1.GroupVersion.String(),
				Kind:       "SleepInfo",
				Name:       sleepInfo.Name,
				UID:        sleepInfo.UID,
			}},
		},
		Data: map[string][]byte{
			lastScheduleKey: []byte(now.Format(time.RFC3339)),
		},
	}

	if resources.HasResource() {
		secret.Data[lastOperationKey] = []byte(data.CurrentOperationType)
	}

	if resources.HasResource() && data.IsSleepOperation() {
		orig, err := resources.GetOriginalInfoToSave()
		if err != nil {
			return fmt.Errorf("get original resource info: %w", err)
		}
		secret.Data[originalJSONPatchDataKey] = orig
	}

	if existing == nil {
		return r.Client.Create(ctx, secret)
	}
	secret.ResourceVersion = existing.ResourceVersion
	return r.Client.Update(ctx, secret)
}
