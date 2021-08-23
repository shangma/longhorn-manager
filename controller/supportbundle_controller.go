package controller

import (
	"fmt"
	"strings"
	"time"

	"github.com/longhorn/longhorn-manager/datastore"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	lhinformers "github.com/longhorn/longhorn-manager/k8s/pkg/client/informers/externalversions/longhorn/v1beta1"
	"github.com/longhorn/longhorn-manager/manager"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"
)

type SupportBundleController struct {
	*baseController

	// which namespace controller is running with
	namespace string
	// use as the OwnerID of the controller
	controllerID string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	sStoreSynced cache.InformerSynced

	SupportBundleInitialized util.Cond

	ServiceAccount string
}

func NewSupportBundleController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	supportBundleInformer lhinformers.SupportBundleInformer,
	kubeClient clientset.Interface,
	controllerID string,
	namespace string,
	serviceAccount string) *SupportBundleController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events(""),
	})

	sbc := &SupportBundleController{
		baseController: newBaseController("longhorn-support-bundle", logger),

		namespace:    namespace,
		controllerID: controllerID,

		ds: ds,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "longhorn-support-bundle-controller"}),

		sStoreSynced:             supportBundleInformer.Informer().HasSynced,
		SupportBundleInitialized: "Initialized",
		ServiceAccount:           serviceAccount,
	}

	supportBundleInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    sbc.enqueueSupportBundle,
		UpdateFunc: func(old, cur interface{}) { sbc.enqueueSupportBundle(cur) },
		DeleteFunc: sbc.enqueueSupportBundle,
	})

	return sbc
}

func (sbc *SupportBundleController) enqueueSupportBundle(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	sbc.queue.AddRateLimited(key)
}

func (sbc *SupportBundleController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer sbc.queue.ShutDown()

	sbc.logger.Infof("Start Longhorn Support Bundle controller")
	defer sbc.logger.Infof("Shutting down Longhorn Support Bundle controller")

	if !cache.WaitForNamedCacheSync(sbc.name, stopCh, sbc.sStoreSynced) {
		return
	}
	for i := 0; i < workers; i++ {
		go wait.Until(sbc.worker, time.Second, stopCh)
	}
	<-stopCh
}

func (sbc *SupportBundleController) worker() {
	for sbc.processNextWorkItem() {
	}
}

func (sbc *SupportBundleController) processNextWorkItem() bool {
	key, quit := sbc.queue.Get()
	if quit {
		return false
	}
	defer sbc.queue.Done(key)
	err := sbc.syncHandler(key.(string))
	sbc.handleErr(err, key)
	return true
}

func (sbc *SupportBundleController) handleErr(err error, key interface{}) {
	if err == nil {
		sbc.queue.Forget(key)
		return
	}

	if sbc.queue.NumRequeues(key) < maxRetries {
		sbc.logger.WithError(err).Warnf("Error syncing Longhorn Support Bundle %v", key)
		sbc.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	sbc.logger.WithError(err).Warnf("Dropping Longhorn Support Bundle %v out of the queue", key)
	sbc.queue.Forget(key)
}

func (sbc *SupportBundleController) syncHandler(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "%v: fail to sync support-bundle %v", sbc.name, key)
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != sbc.namespace {
		return nil
	}
	return sbc.reconcile(name)
}

func getLoggerForSupportBundle(logger logrus.FieldLogger, supportBundle *longhorn.SupportBundle) *logrus.Entry {
	return logger.WithFields(
		logrus.Fields{
			"support-bundle": supportBundle.Name,
		},
	)
}

func (sbc *SupportBundleController) reconcile(supportBundleName string) (err error) {
	// Get SupportBundle CR
	sb, err := sbc.ds.GetSupportBundle(supportBundleName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	if sb == nil || sb.DeletionTimestamp != nil {
		return nil
	}

	log := getLoggerForSupportBundle(sbc.logger, sb)

	switch sb.Status.State {
	case types.SupportBundleStateNone:
		log.Debugf("[%s] generating a support bundle", sb.Name)

		supportBundleImage, err := sbc.ds.GetSetting("SettingNameSupportBundleImage")
		if err != nil {
			return err
		}
		err = sbc.Create(sb, supportBundleImage.Value)
		toUpdate := sb.DeepCopy()
		if err != nil {
			sbc.setError(toUpdate, fmt.Sprintf("fail to create manager for %s: %s", sb.Name, err))
			return err
		}
		sbc.setState(toUpdate, types.SupportBundleStateGenerating)
		_, err = sbc.ds.UpdateSupportBundle(toUpdate)
		return err
	default:
		log.Debugf("[%s] noop for state %s", sb.Name, sb.Status.State)
		return nil
	}

}
func (sbc *SupportBundleController) getManagerName(supportBundle *longhorn.SupportBundle) string {
	return fmt.Sprintf("supportbundle-manager-%s", supportBundle.Name)
}

func (sbc *SupportBundleController) getImagePullPolicy() corev1.PullPolicy {

	imagePullPolicy, err := sbc.ds.GetSetting(types.SettingNameSystemManagedPodsImagePullPolicy)
	if err != nil {
		return corev1.PullIfNotPresent
	}

	switch strings.ToLower(imagePullPolicy.Value) {
	case "always":
		return corev1.PullAlways
	case "if-not-present":
		return corev1.PullIfNotPresent
	case "never":
		return corev1.PullNever
	default:
		return corev1.PullIfNotPresent
	}
}

func (sbc *SupportBundleController) Create(sb *longhorn.SupportBundle, image string) error {

	log := getLoggerForSupportBundle(sbc.logger, sb)

	deployName := sbc.getManagerName(sb)
	log.Debugf("creating deployment %s with image %s", deployName, image)

	pullPolicy := sbc.getImagePullPolicy()

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: sb.Namespace,
			Labels: map[string]string{
				"app":                       types.SupportBundleManager,
				types.SupportBundleLabelKey: sb.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       sb.Name,
					Kind:       sb.Kind,
					UID:        sb.UID,
					APIVersion: sb.APIVersion,
				},
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": types.SupportBundleManager},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                       types.SupportBundleManager,
						types.SupportBundleLabelKey: sb.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "manager",
							Image:           image,
							Args:            []string{"/usr/bin/support-bundle-utils", "manager"},
							ImagePullPolicy: pullPolicy,
							Env: []corev1.EnvVar{
								{
									Name:  "LONGHORN_NAMESPACE",
									Value: sb.Namespace,
								},
								{
									Name:  "LONGHORN_VERSION",
									Value: manager.FriendlyVersion(),
								},
								{
									Name:  "LONGHORN_SUPPORT_BUNDLE_NAME",
									Value: sb.Name,
								},
								{
									Name:  "LONGHORN_SUPPORT_BUNDLE_DEBUG",
									Value: "true",
								},
								{
									Name: "LONGHORN_SUPPORT_BUNDLE_MANAGER_POD_IP",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "status.podIP",
										},
									},
								},
								{
									Name:  "LONGHORN_SUPPORT_BUNDLE_IMAGE",
									Value: image,
								},
								{
									Name:  "LONGHORN_SUPPORT_BUNDLE_IMAGE_PULL_POLICY",
									Value: string(pullPolicy),
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
								},
							},
						},
					},
					ServiceAccountName: sbc.ServiceAccount,
				},
			},
		},
	}

	_, err := sbc.ds.CreateDeployment(deployment)
	return err
}

func (sbc *SupportBundleController) setError(toUpdate *longhorn.SupportBundle, reason string) {
	log := getLoggerForSupportBundle(sbc.logger, toUpdate)
	log.Errorf(reason)

	sbc.SupportBundleInitialized.False(toUpdate)
	sbc.SupportBundleInitialized.Message(toUpdate, reason)

	toUpdate.Status.State = types.SupportBundleStateError
}

func (sbc *SupportBundleController) setState(toUpdate *longhorn.SupportBundle, state string) {
	log := getLoggerForSupportBundle(sbc.logger, toUpdate)
	log.Debugf("[%s] set state to %s", toUpdate.Name, state)

	if state == types.SupportBundleStateReady {
		log.Debugf("[%s] set condition %s to true", toUpdate.Name, sbc.SupportBundleInitialized)
		sbc.SupportBundleInitialized.True(toUpdate)
	}

	toUpdate.Status.State = state
}
