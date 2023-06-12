package main

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2/ktesting"

	samplecontroller "custom-hpa/pkg/apis/customhpa/v1alpha1"
	"custom-hpa/pkg/generated/clientset/versioned/fake"
	informers "custom-hpa/pkg/generated/informers/externalversions"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

type fixture struct {
	t *testing.T

	client     *fake.Clientset
	kubeclient *k8sfake.Clientset
	// Objects to put in the store.
	customhpaLister               []*samplecontroller.CustomHPA
	horizontalPodAutoscalerLister []*autoscalingv2.HorizontalPodAutoscaler
	// Actions expected to happen on the client.
	kubeactions []core.Action
	actions     []core.Action
	// Objects from here preloaded into NewSimpleFake.
	kubeobjects []runtime.Object
	objects     []runtime.Object
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{}
	f.t = t
	f.objects = []runtime.Object{}
	f.kubeobjects = []runtime.Object{}
	return f
}

func newCustomHPA(name string, replicas *int32) *samplecontroller.CustomHPA {
	return &samplecontroller.CustomHPA{
		TypeMeta: metav1.TypeMeta{APIVersion: samplecontroller.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: samplecontroller.CustomHPASpec{
			HorizontalPodAutoscalerName: fmt.Sprintf("%s-custom-hpa", name),
			MinReplicas:                 replicas,
		},
	}
}

func (f *fixture) newController(ctx context.Context) (*Controller, informers.SharedInformerFactory, kubeinformers.SharedInformerFactory) {
	f.client = fake.NewSimpleClientset(f.objects...)
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)

	i := informers.NewSharedInformerFactory(f.client, noResyncPeriodFunc())
	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())

	c := NewController(ctx, f.kubeclient, f.client,
		k8sI.Autoscaling().V2().HorizontalPodAutoscalers(), i.Customhpa().V1alpha1().CustomHPAs())

	c.customhpasSynced = alwaysReady
	c.holizontalPodAutoscalerSynced = alwaysReady
	c.recorder = &record.FakeRecorder{}

	for _, f := range f.customhpaLister {
		i.Customhpa().V1alpha1().CustomHPAs().Informer().GetIndexer().Add(f)
	}

	for _, d := range f.horizontalPodAutoscalerLister {
		k8sI.Autoscaling().V2().HorizontalPodAutoscalers().Informer().GetIndexer().Add(d)
	}

	return c, i, k8sI
}

func (f *fixture) run(ctx context.Context, customhpaName string) {
	f.runController(ctx, customhpaName, true, false)
}

func (f *fixture) runExpectError(ctx context.Context, customhpaName string) {
	f.runController(ctx, customhpaName, true, true)
}

func (f *fixture) runController(ctx context.Context, customhpaName string, startInformers bool, expectError bool) {
	c, i, k8sI := f.newController(ctx)
	if startInformers {
		i.Start(ctx.Done())
		k8sI.Start(ctx.Done())
	}

	err := c.syncHandler(ctx, customhpaName)
	if !expectError && err != nil {
		f.t.Errorf("error syncing customhpa: %v", err)
	} else if expectError && err == nil {
		f.t.Error("expected error syncing customhpa, got nil")
	}

	actions := filterInformerActions(f.client.Actions())
	for i, action := range actions {
		if len(f.actions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(actions)-len(f.actions), actions[i:])
			break
		}

		expectedAction := f.actions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.actions) > len(actions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.actions)-len(actions), f.actions[len(actions):])
	}

	k8sActions := filterInformerActions(f.kubeclient.Actions())
	for i, action := range k8sActions {
		if len(f.kubeactions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(k8sActions)-len(f.kubeactions), k8sActions[i:])
			break
		}

		expectedAction := f.kubeactions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.kubeactions) > len(k8sActions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.kubeactions)-len(k8sActions), f.kubeactions[len(k8sActions):])
	}
}

// checkAction verifies that expected and actual actions are equal and both have
// same attached resources
func checkAction(expected, actual core.Action, t *testing.T) {
	if !(expected.Matches(actual.GetVerb(), actual.GetResource().Resource) && actual.GetSubresource() == expected.GetSubresource()) {
		t.Errorf("Expected\n\t%#v\ngot\n\t%#v", expected, actual)
		return
	}

	if reflect.TypeOf(actual) != reflect.TypeOf(expected) {
		t.Errorf("Action has wrong type. Expected: %t. Got: %t", expected, actual)
		return
	}

	switch a := actual.(type) {
	case core.CreateActionImpl:
		e, _ := expected.(core.CreateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.UpdateActionImpl:
		e, _ := expected.(core.UpdateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.PatchActionImpl:
		e, _ := expected.(core.PatchActionImpl)
		expPatch := e.GetPatch()
		patch := a.GetPatch()

		if !reflect.DeepEqual(expPatch, patch) {
			t.Errorf("Action %s %s has wrong patch\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expPatch, patch))
		}
	default:
		t.Errorf("Uncaptured Action %s %s, you should explicitly add a case to capture it",
			actual.GetVerb(), actual.GetResource().Resource)
	}
}

// filterInformerActions filters list and watch actions for testing resources.
// Since list and watch don't change resource state we can filter it to lower
// nose level in our tests.
func filterInformerActions(actions []core.Action) []core.Action {
	ret := []core.Action{}
	for _, action := range actions {
		if len(action.GetNamespace()) == 0 &&
			(action.Matches("list", "customhpas") ||
				action.Matches("watch", "customhpas") ||
				action.Matches("list", "horizontalpodautoscalers") ||
				action.Matches("watch", "horizontalpodautoscalers")) {
			continue
		}
		ret = append(ret, action)
	}

	return ret
}

func (f *fixture) expectCreateHorizontalPodAutoscalerAction(hpa *autoscalingv2.HorizontalPodAutoscaler) {
	f.kubeactions = append(f.kubeactions, core.NewCreateAction(schema.GroupVersionResource{Resource: "horizontalpodautoscalers"}, hpa.Namespace, hpa))
}

func (f *fixture) expectUpdateHorizontalPodAutoscalerAction(hpa *autoscalingv2.HorizontalPodAutoscaler) {
	f.kubeactions = append(f.kubeactions, core.NewUpdateAction(schema.GroupVersionResource{Resource: "horizontalpodautoscalers"}, hpa.Namespace, hpa))
}

func (f *fixture) expectUpdateCustomHPAStatusAction(customhpa *samplecontroller.CustomHPA) {
	action := core.NewUpdateSubresourceAction(schema.GroupVersionResource{Resource: "customhpas"}, "status", customhpa.Namespace, customhpa)
	f.actions = append(f.actions, action)
}

func getKey(customhpa *samplecontroller.CustomHPA, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(customhpa)
	if err != nil {
		t.Errorf("Unexpected error getting key for customhpa %v: %v", customhpa.Name, err)
		return ""
	}
	return key
}

func TestCreatesHorizontalPodAutoscaler(t *testing.T) {
	f := newFixture(t)
	customhpa := newCustomHPA("test", int32Ptr(1))
	_, ctx := ktesting.NewTestContext(t)

	f.customhpaLister = append(f.customhpaLister, customhpa)
	f.objects = append(f.objects, customhpa)

	expHorizontalPodAutoscaler := newHorizontalPodAutoscaler(customhpa)
	f.expectCreateHorizontalPodAutoscalerAction(expHorizontalPodAutoscaler)
	f.expectUpdateCustomHPAStatusAction(customhpa)

	f.run(ctx, getKey(customhpa, t))
}

func TestDoNothing(t *testing.T) {
	f := newFixture(t)
	customhpa := newCustomHPA("test", int32Ptr(1))
	_, ctx := ktesting.NewTestContext(t)

	d := newHorizontalPodAutoscaler(customhpa)

	f.customhpaLister = append(f.customhpaLister, customhpa)
	f.objects = append(f.objects, customhpa)
	f.horizontalPodAutoscalerLister = append(f.horizontalPodAutoscalerLister, d)
	f.kubeobjects = append(f.kubeobjects, d)

	f.expectUpdateCustomHPAStatusAction(customhpa)
	f.run(ctx, getKey(customhpa, t))
}

func TestUpdateHorizontalPodAutoscaler(t *testing.T) {
	f := newFixture(t)
	customhpa := newCustomHPA("test", int32Ptr(1))
	_, ctx := ktesting.NewTestContext(t)

	d := newHorizontalPodAutoscaler(customhpa)

	// Update replicas
	customhpa.Spec.MinReplicas = int32Ptr(2)
	expHorizontalPodAutoscaler := newHorizontalPodAutoscaler(customhpa)

	f.customhpaLister = append(f.customhpaLister, customhpa)
	f.objects = append(f.objects, customhpa)
	f.horizontalPodAutoscalerLister = append(f.horizontalPodAutoscalerLister, d)
	f.kubeobjects = append(f.kubeobjects, d)

	f.expectUpdateCustomHPAStatusAction(customhpa)
	f.expectUpdateHorizontalPodAutoscalerAction(expHorizontalPodAutoscaler)
	f.run(ctx, getKey(customhpa, t))
}

func TestNotControlledByUs(t *testing.T) {
	f := newFixture(t)
	customhpa := newCustomHPA("test", int32Ptr(1))
	_, ctx := ktesting.NewTestContext(t)

	d := newHorizontalPodAutoscaler(customhpa)

	d.ObjectMeta.OwnerReferences = []metav1.OwnerReference{}

	f.customhpaLister = append(f.customhpaLister, customhpa)
	f.objects = append(f.objects, customhpa)
	f.horizontalPodAutoscalerLister = append(f.horizontalPodAutoscalerLister, d)
	f.kubeobjects = append(f.kubeobjects, d)

	f.runExpectError(ctx, getKey(customhpa, t))
}

func int32Ptr(i int32) *int32 { return &i }
