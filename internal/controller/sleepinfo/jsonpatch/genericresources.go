package jsonpatch

import (
	"context"
	"fmt"
	"strings"

	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/resource"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type genericResource struct {
	resource.ResourceClient
	patchData      v1alpha1.Patch
	restorePatches RestorePatches
	data           []unstructured.Unstructured
	// FIXME:
	// this cache parameter is used to simplify the implementation (avoiding to repeat
	// some error done in other resource implementation managing data) without change
	// the basic controller logic and without performance issues (it avoids to useless refetch
	// 2 times the resources).
	// This implementation should be improved.
	isCacheInvalid bool
}

func newGenericResource(res resource.ResourceClient, patchData v1alpha1.Patch, restorePatches RestorePatches) *genericResource {
	return &genericResource{
		ResourceClient: res,
		patchData:      patchData,
		restorePatches: restorePatches,
	}
}

func (c genericResource) getListByNamespace(ctx context.Context, namespace string, target v1alpha1.PatchTarget) ([]unstructured.Unstructured, error) {
	// TODO: manage optional version. So it will be possible to manage also multiple
	// version of the same resource
	restMapping, err := c.Client.RESTMapper().RESTMapping(target.GroupKind())
	if meta.IsNoMatchError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.ResourceClient.Log.V(8).Info("prepare resources list", "target", target, "gvk", restMapping.GroupVersionKind.String())

	resourceList := unstructured.UnstructuredList{}
	resourceList.SetGroupVersionKind(restMapping.GroupVersionKind)

	listOptions, namesToInclude, err := c.getListOptions(namespace, target)
	if err != nil {
		return nil, err
	}

	if err := c.Client.List(ctx, &resourceList, listOptions); err != nil {
		return resourceList.Items, client.IgnoreNotFound(err)
	}

	items := filterByName(resourceList.Items, namesToInclude)

	c.Log.V(8).Info("resources list", "gvk", restMapping.GroupVersionKind.String(), "length", len(items))

	return items, nil
}

// filterByName keeps only the resources listed in namesToInclude, and is a no-op
// when it is empty ,  it completes the include by name filter for the cases which
// cannot be expressed with a field selector, see getFieldToInclude.
func filterByName(items []unstructured.Unstructured, namesToInclude map[string]struct{}) []unstructured.Unstructured {
	if len(namesToInclude) == 0 {
		return items
	}

	filtered := make([]unstructured.Unstructured, 0, len(items))
	for _, item := range items {
		if _, ok := namesToInclude[item.GetName()]; ok {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// getListOptions returns the options to list the target resources the returned
// map, when not empty, contains the names to keep once the list is fetched.
func (g genericResource) getListOptions(namespace string, target v1alpha1.PatchTarget) (*client.ListOptions, map[string]struct{}, error) {
	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}

	includeRef := g.SleepInfo.GetIncludeRef()
	excludeRef := g.SleepInfo.GetExcludeRef()
	fieldsToInclude, namesToInclude := getFieldToInclude(includeRef, target)
	labelsToInclude := getLabelsToInclude(includeRef)
	fieldsToExclude := getFieldToExclude(excludeRef, target)
	labelsToExclude := getLabelsToExclude(excludeRef)

	// Combine fields to include and exclude into a single field selector
	var fieldSelectors []string
	fieldSelectors = append(fieldSelectors, fieldsToInclude...)
	fieldSelectors = append(fieldSelectors, fieldsToExclude...)

	if len(fieldSelectors) > 0 {
		fieldSelector, err := fields.ParseSelector(strings.Join(fieldSelectors, ","))
		if err != nil {
			return nil, nil, err
		}
		listOptions.FieldSelector = fieldSelector
	}

	// Combine labels to include and exclude into a single label selector
	var labelSelectors []string
	labelSelectors = append(labelSelectors, labelsToInclude...)
	labelSelectors = append(labelSelectors, labelsToExclude...)

	if len(labelSelectors) > 0 {
		labelSelector, err := labels.Parse(strings.Join(labelSelectors, ","))
		if err != nil {
			return nil, nil, err
		}
		listOptions.LabelSelector = labelSelector
	}

	return listOptions, namesToInclude, nil
}

func getFieldToExclude(excludeRef []v1alpha1.FilterRef, target v1alpha1.PatchTarget) []string {
	var names []string
	for _, exclude := range excludeRef {
		if matchPatchTargetAndFilterRef(target, exclude) && exclude.Name != "" {
			names = append(names, exclude.Name)
		}
	}

	fieldsSelector := []string{}
	for _, name := range names {
		fieldsSelector = append(fieldsSelector, fmt.Sprintf("metadata.name!=%s", name))
	}
	return fieldsSelector
}

// getFieldToInclude returns the field selectors to include the target resources by
// name. Field selectors are evaluated in AND, so more than one name for the same
// target cannot be expressed as a selector: those names are returned instead, to be
// filtered once the list is fetched.
func getFieldToInclude(includeRef []v1alpha1.FilterRef, target v1alpha1.PatchTarget) ([]string, map[string]struct{}) {
	var names []string
	for _, include := range includeRef {
		if matchPatchTargetAndFilterRef(target, include) && include.Name != "" {
			names = append(names, include.Name)
		}
	}

	switch len(names) {
	case 0:
		return nil, nil
	case 1:
		return []string{fmt.Sprintf("metadata.name==%s", names[0])}, nil
	default:
		namesToInclude := make(map[string]struct{}, len(names))
		for _, name := range names {
			namesToInclude[name] = struct{}{}
		}
		return nil, namesToInclude
	}
}

// TODO: check when add support to versions
func matchPatchTargetAndFilterRef(target v1alpha1.PatchTarget, filterRef v1alpha1.FilterRef) bool {
	return strings.HasPrefix(filterRef.APIVersion, fmt.Sprintf("%s/", target.Group)) && filterRef.Kind == target.Kind
}

func getLabelsToExclude(excludeRef []v1alpha1.FilterRef) []string {
	labelsToExclude := []string{}
	for _, exclude := range excludeRef {
		for k, v := range exclude.MatchLabels {
			labelsToExclude = append(labelsToExclude, fmt.Sprintf("%s!=%s", k, v))
		}
	}
	return labelsToExclude
}

func getLabelsToInclude(includeRef []v1alpha1.FilterRef) []string {
	labelsToInclude := []string{}
	for _, include := range includeRef {
		for k, v := range include.MatchLabels {
			labelsToInclude = append(labelsToInclude, fmt.Sprintf("%s==%s", k, v))
		}
	}
	return labelsToInclude
}
