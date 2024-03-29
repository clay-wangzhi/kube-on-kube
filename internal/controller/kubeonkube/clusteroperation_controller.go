/*
Copyright 2024 Clay.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kubeonkube

import (
	"context"
	"crypto/md5"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	klog "k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kube-on-kube/api"
	kubeonkubev1alpha1 "kube-on-kube/api/kubeonkube/v1alpha1"
	kokClientSet "kube-on-kube/generated/clientset/versioned"
	"kube-on-kube/pkg/util"
	"kube-on-kube/pkg/util/entrypoint"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	requeueAfter    = time.Second * 3
	BaseSlat        = "kubeonkube"
	ClusterLabelKey = "clusterName"
	ServiceAccount  = "clay.io/kubeonkube-operator=sa"
	SprayJobPodName = "kubeonkube"
)

// ClusterOperationReconciler reconciles a ClusterOperation object
type ClusterOperationReconciler struct {
	Client       client.Client
	Scheme       *runtime.Scheme
	ClientSet    kubernetes.Interface
	KokClientSet kokClientSet.Interface
}

//+kubebuilder:rbac:groups=kubeonkube.clay.io,resources=clusteroperations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubeonkube.clay.io,resources=clusteroperations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubeonkube.clay.io,resources=clusteroperations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ClusterOperation object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *ClusterOperationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	clusterOps := &kubeonkubev1alpha1.ClusterOperation{}
	if err := r.Client.Get(ctx, req.NamespacedName, clusterOps); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		klog.ErrorS(err, "failed to get cluster ops", "clusterOps", req.Name)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	// stop reconcile if the clusterOps has been already finished
	if clusterOps.Status.Status == kubeonkubev1alpha1.SucceededStatus || clusterOps.Status.Status == kubeonkubev1alpha1.FailedStatus {
		return ctrl.Result{}, nil
	}

	// 从 cluster 中获取一些必要信息
	cluster, err := r.GetKubeOnkubeCluster(clusterOps)
	if err != nil {
		klog.ErrorS(err, "failed to get kubeonkube cluster", "clusterOps", clusterOps.Name)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// 判断镜像名称是否合理, 镜像不合理就将状态设置为失败，终止调谐
	if !IsValidImageName(clusterOps.Spec.Image) {
		klog.Errorf("clusterOps %s has wrong image format and update status Failed", clusterOps.Name)
		clusterOps.Status.Status = kubeonkubev1alpha1.FailedStatus
		if err := r.Client.Status().Update(ctx, clusterOps); err != nil {
			klog.Error(err)
		}
		return ctrl.Result{}, nil
	}

	// 检查相关配置文件是否存在,不存在设置为失败，终止调谐
	if err := r.CheckClusterDataRef(cluster, clusterOps); err != nil {
		klog.Error(err.Error())
		clusterOps.Status.Status = kubeonkubev1alpha1.FailedStatus
		if err := r.Client.Status().Update(ctx, clusterOps); err != nil {
			klog.Error(err)
		}
		return ctrl.Result{}, nil
	}

	// 添加 OwnReference, 然后延迟加入队列，继续调谐
	needRequeue, err := r.UpdateOperationOwnReferenceForCluster(cluster, clusterOps)
	if err != nil {
		klog.ErrorS(err, "failed to update ownreference", "cluster", cluster.Name, "clusterOps", clusterOps.Name)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	if needRequeue {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// 更新 StatusDigest，然后延迟加入队列，继续调谐
	needRequeue, err = r.UpdateClusterOpsStatusDigest(clusterOps)
	if err != nil {
		klog.ErrorS(err, "failed to get update clusterOps status digest", "clusterOps", clusterOps.Name)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	if needRequeue {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// 根基 Digest 判断 Yaml 文件创建后是否被修改过, 如果修改过，更新状态，然后延迟加入队列，继续调谐
	needRequeue, err = r.UpdateStatusHasModified(clusterOps)
	if err != nil {
		klog.ErrorS(err, "failed to update clusterOps status", "clusterOps", clusterOps.Name)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	if needRequeue {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// 拷贝会用到的配置文件
	needRequeue, err = r.BackUpDataRef(clusterOps, cluster)
	if err != nil {
		klog.ErrorS(err, "failed to backup data ref", "clusterOps", clusterOps.Name)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	if needRequeue {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// 生成 entrypoint 命令,存入 configmap中
	needRequeue, err = r.CreateEntryPointShellConfigMap(clusterOps)
	if argsErr, ok := err.(entrypoint.ArgsError); ok {
		// preHook or postHook or action error args
		klog.Errorf("clusterOps %s wrong args %s and update status Failed", clusterOps.Name, argsErr.Error())
		clusterOps.Status.Status = kubeonkubev1alpha1.FailedStatus
		if err := r.Client.Status().Update(ctx, clusterOps); err != nil {
			klog.Error(err)
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	if err != nil {
		klog.ErrorS(err, "failed to create entrypoint shell configmap", "clusterOps", clusterOps.Name)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	if needRequeue {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// 新增 Job
	needRequeue, err = r.CreateKubeSprayJob(clusterOps)
	if err != nil {
		klog.ErrorS(err, "failed to create kubespray job", "clusterOps", clusterOps.Name)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	if needRequeue {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// 更新状态
	needRequeue, err = r.UpdateStatusLoop(clusterOps, r.FetchJobConditionStatusAndCompletionTime)
	if err != nil {
		klog.ErrorS(err, "failed to update status loop", "clusterOps", clusterOps.Name)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	if needRequeue {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	if err := r.UpdateStatusForLabel(clusterOps); err != nil {
		klog.Error(err)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterOperationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubeonkubev1alpha1.ClusterOperation{}).
		Complete(r)
}

func (r *ClusterOperationReconciler) GetKubeOnkubeCluster(clusterOps *kubeonkubev1alpha1.ClusterOperation) (*kubeonkubev1alpha1.Cluster, error) {
	// cluster has many clusterOps.
	return r.KokClientSet.KubeonkubeV1alpha1().Clusters().Get(context.Background(), clusterOps.Spec.Cluster, metav1.GetOptions{})
}

func IsValidImageName(image string) bool {
	isNumberOrLetter := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsNumber(r)
	}
	if len(image) == 0 || strings.Contains(image, " ") {
		return false
	}
	runSlice := []rune(image)
	return isNumberOrLetter(runSlice[0]) && isNumberOrLetter(runSlice[len(runSlice)-1])
}

// 检查 Cluster 中是否存在配置文件
func (r *ClusterOperationReconciler) CheckClusterDataRef(cluster *kubeonkubev1alpha1.Cluster, clusterOps *kubeonkubev1alpha1.ClusterOperation) error {
	// 判断是否文件是否在同一个 namespace 内
	namespaceSet := map[string]struct{}{}
	if clusterOps.Spec.HostsConfRef.IsEmpty() {
		// 检查 cluster 中是否存在
		hostsConfRef := cluster.Spec.HostsConfRef
		if hostsConfRef.IsEmpty() {
			return fmt.Errorf("Cluster %s hostsConfRef is empty", cluster.Name)
		}
		if !r.CheckConfigMapExist(hostsConfRef.NameSpace, hostsConfRef.Name) {
			return fmt.Errorf("Cluster %s hostsConfRef %s,%s not found", cluster.Name, hostsConfRef.NameSpace, hostsConfRef.Name)
		}
		namespaceSet[hostsConfRef.NameSpace] = struct{}{}
	}
	if clusterOps.Spec.VarsConfRef.IsEmpty() {
		varsConfRef := cluster.Spec.VarsConfRef
		if varsConfRef.IsEmpty() {
			return fmt.Errorf("Cluster %s varsConfRef is empty", cluster.Name)
		}
		if !r.CheckConfigMapExist(varsConfRef.NameSpace, varsConfRef.Name) {
			return fmt.Errorf("Cluster %s varsConfRef %s,%s not found", cluster.Name, varsConfRef.NameSpace, varsConfRef.Name)
		}
		namespaceSet[varsConfRef.NameSpace] = struct{}{}
	}
	if clusterOps.Spec.SSHAuthRef.IsEmpty() && !cluster.Spec.SSHAuthRef.IsEmpty() {
		// check SSHAuthRef optionally.
		sshAuthRef := cluster.Spec.SSHAuthRef
		if !r.CheckSecretExist(sshAuthRef.NameSpace, sshAuthRef.Name) {
			return fmt.Errorf("Cluster %s sshAuthRef %s,%s not found", cluster.Name, sshAuthRef.NameSpace, sshAuthRef.Name)
		}
		namespaceSet[sshAuthRef.NameSpace] = struct{}{}
	}
	if len(namespaceSet) > 1 {
		return fmt.Errorf("Cluster %s hostsConfRef varsConfRef or sshAuthRef not in the same namespace", cluster.Name)
	}
	return nil
}

func (r *ClusterOperationReconciler) CheckConfigMapExist(namespace, name string) bool {
	if _, err := r.ClientSet.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{}); err != nil && apierrors.IsNotFound(err) {
		return false
	}
	return true
}

func (r *ClusterOperationReconciler) CheckSecretExist(namespace, name string) bool {
	if _, err := r.ClientSet.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{}); err != nil && apierrors.IsNotFound(err) {
		return false
	}
	return true
}

func (r *ClusterOperationReconciler) UpdateOperationOwnReferenceForCluster(cluster *kubeonkubev1alpha1.Cluster, clusterOps *kubeonkubev1alpha1.ClusterOperation) (bool, error) {
	for i := range clusterOps.OwnerReferences {
		// 已经设置过了
		if clusterOps.OwnerReferences[i].UID == cluster.UID {
			return false, nil
		}
	}
	clusterOps.OwnerReferences = append(clusterOps.OwnerReferences, *metav1.NewControllerRef(cluster, kubeonkubev1alpha1.SchemeGroupVersion.WithKind("Cluster")))
	if err := r.Client.Update(context.Background(), clusterOps); err != nil {
		return false, err
	}
	return true, nil
}

func (r *ClusterOperationReconciler) UpdateClusterOpsStatusDigest(clusterOps *kubeonkubev1alpha1.ClusterOperation) (bool, error) {
	if len(clusterOps.Status.Digest) != 0 {
		// 已经设置过了
		return false, nil
	}
	// 初始化赋值
	clusterOps.Status.Digest = r.CalSalt(clusterOps)
	if err := r.Client.Status().Update(context.Background(), clusterOps); err != nil {
		return false, err
	}
	return true, nil
}

func (r *ClusterOperationReconciler) CalSalt(clusterOps *kubeonkubev1alpha1.ClusterOperation) string {
	summaryStr := ""
	summaryStr += BaseSlat
	summaryStr += clusterOps.Spec.Cluster
	summaryStr += string(clusterOps.Spec.ActionType)
	summaryStr += strings.TrimSpace(clusterOps.Spec.Action)
	summaryStr += clusterOps.Spec.Image
	for _, action := range clusterOps.Spec.PreHook {
		summaryStr += string(action.ActionType)
		summaryStr += strings.TrimSpace(action.Action)
	}
	for _, action := range clusterOps.Spec.PostHook {
		summaryStr += string(action.ActionType)
		summaryStr += strings.TrimSpace(action.Action)
	}
	return fmt.Sprintf("%x", md5.Sum([]byte(summaryStr)))
}

func (r *ClusterOperationReconciler) UpdateStatusHasModified(clusterOps *kubeonkubev1alpha1.ClusterOperation) (bool, error) {
	if len(clusterOps.Status.Digest) == 0 {
		return false, nil
	}
	if clusterOps.Status.HasModified {
		// 已经设置过了
		return false, nil
	}
	if same := r.compareDigest(clusterOps); !same {
		// 不同，则更新状态
		clusterOps.Status.HasModified = true
		if err := r.Client.Status().Update(context.Background(), clusterOps); err != nil {
			return false, err
		}
		klog.Warningf("clusterOps %s Spec has been modified", clusterOps.Name)
		return true, nil
	}
	return false, nil
}

func (r *ClusterOperationReconciler) compareDigest(clusterOps *kubeonkubev1alpha1.ClusterOperation) bool {
	return clusterOps.Status.Digest == r.CalSalt(clusterOps)
}

// 执行配置文件的备份，浅拷贝，去使用
func (r *ClusterOperationReconciler) BackUpDataRef(clusterOps *kubeonkubev1alpha1.ClusterOperation, cluster *kubeonkubev1alpha1.Cluster) (bool, error) {
	timestamp := fmt.Sprintf("-%d", time.Now().UnixMilli())
	if cluster.Spec.HostsConfRef.IsEmpty() || cluster.Spec.VarsConfRef.IsEmpty() {
		return false, fmt.Errorf("cluster %s DataRef has empty value", cluster.Name)
	}
	if clusterOps.Labels == nil {
		clusterOps.Labels = map[string]string{ClusterLabelKey: cluster.Name}
	} else {
		clusterOps.Labels[ClusterLabelKey] = cluster.Name
	}
	currentNS := util.GetCurrentNSOrDefault()
	if clusterOps.Spec.HostsConfRef.IsEmpty() {
		newConfigMap, err := r.CopyConfigMap(clusterOps, cluster.Spec.HostsConfRef, cluster.Spec.HostsConfRef.Name+timestamp, currentNS)
		if err != nil {
			return false, err
		}
		clusterOps.Spec.HostsConfRef = &api.ConfigMapRef{
			NameSpace: newConfigMap.Namespace,
			Name:      newConfigMap.Name,
		}
		if err := r.Client.Update(context.Background(), clusterOps); err != nil {
			return false, err
		}
		return true, nil
	}
	if clusterOps.Spec.VarsConfRef.IsEmpty() {
		newConfigMap, err := r.CopyConfigMap(clusterOps, cluster.Spec.VarsConfRef, cluster.Spec.VarsConfRef.Name+timestamp, currentNS)
		if err != nil {
			return false, err
		}
		clusterOps.Spec.VarsConfRef = &api.ConfigMapRef{
			NameSpace: newConfigMap.Namespace,
			Name:      newConfigMap.Name,
		}
		if err := r.Client.Update(context.Background(), clusterOps); err != nil {
			return false, err
		}
		return true, nil
	}
	if clusterOps.Spec.SSHAuthRef.IsEmpty() && !cluster.Spec.SSHAuthRef.IsEmpty() {
		// clusterOps backups ssh data when cluster has ssh data.
		newSecret, err := r.CopySecret(clusterOps, cluster.Spec.SSHAuthRef, cluster.Spec.SSHAuthRef.Name+timestamp, currentNS)
		if err != nil {
			return false, err
		}
		clusterOps.Spec.SSHAuthRef = &api.SecretRef{
			NameSpace: newSecret.Namespace,
			Name:      newSecret.Name,
		}
		if err := r.Client.Update(context.Background(), clusterOps); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// 拷贝配置文件
func (r *ClusterOperationReconciler) CopyConfigMap(clusterOps *kubeonkubev1alpha1.ClusterOperation, oldConfigMapRef *api.ConfigMapRef, newName, newNamespace string) (*corev1.ConfigMap, error) {
	oldConfigMap, err := r.ClientSet.CoreV1().ConfigMaps(oldConfigMapRef.NameSpace).Get(context.Background(), oldConfigMapRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	namespace := oldConfigMapRef.NameSpace
	if newNamespace != "" {
		namespace = newNamespace
	}
	newConfigMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      newName,
			Namespace: namespace,
		},
		Data: oldConfigMap.Data,
	}
	r.SetOwnerReferences(&newConfigMap.ObjectMeta, clusterOps)
	newConfigMap, err = r.ClientSet.CoreV1().ConfigMaps(newConfigMap.Namespace).Create(context.Background(), newConfigMap, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return newConfigMap, nil
}

// 拷贝 Secret
func (r *ClusterOperationReconciler) CopySecret(clusterOps *kubeonkubev1alpha1.ClusterOperation, oldSecretRef *api.SecretRef, newName, newNamespace string) (*corev1.Secret, error) {
	oldSecret, err := r.ClientSet.CoreV1().Secrets(oldSecretRef.NameSpace).Get(context.Background(), oldSecretRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	namespace := oldSecretRef.NameSpace
	if newNamespace != "" {
		namespace = newNamespace
	}
	newSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      newName,
			Namespace: namespace,
		},
		Data: oldSecret.Data,
	}
	r.SetOwnerReferences(&newSecret.ObjectMeta, clusterOps)
	newSecret, err = r.ClientSet.CoreV1().Secrets(newSecret.Namespace).Create(context.Background(), newSecret, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return newSecret, nil
}

func (r *ClusterOperationReconciler) SetOwnerReferences(objectMetaData *metav1.ObjectMeta, clusterOps *kubeonkubev1alpha1.ClusterOperation) {
	objectMetaData.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(clusterOps, kubeonkubev1alpha1.SchemeGroupVersion.WithKind("ClusterOperation"))}
}

// CreateEntryPointShellConfigMap create configMap to store entrypoint.sh.
func (r *ClusterOperationReconciler) CreateEntryPointShellConfigMap(clusterOps *kubeonkubev1alpha1.ClusterOperation) (bool, error) {
	if !clusterOps.Spec.EntrypointSHRef.IsEmpty() {
		return false, nil
	}
	entryPointData := entrypoint.NewEntryPoint()
	isPrivateKey := !clusterOps.Spec.SSHAuthRef.IsEmpty()
	builtinActionSource := kubeonkubev1alpha1.BuiltinActionSource
	for _, action := range clusterOps.Spec.PreHook {
		if err := entryPointData.PreHookRunPart(string(action.ActionType), action.Action, action.ExtraArgs, isPrivateKey, action.ActionSource == nil || *action.ActionSource == builtinActionSource); err != nil {
			return false, err
		}
	}
	if err := entryPointData.SprayRunPart(string(clusterOps.Spec.ActionType), clusterOps.Spec.Action, clusterOps.Spec.ExtraArgs, isPrivateKey, clusterOps.Spec.ActionSource == nil || *clusterOps.Spec.ActionSource == builtinActionSource); err != nil {
		return false, err
	}
	for _, action := range clusterOps.Spec.PostHook {
		if err := entryPointData.PostHookRunPart(string(action.ActionType), action.Action, action.ExtraArgs, isPrivateKey, action.ActionSource == nil || *action.ActionSource == builtinActionSource); err != nil {
			return false, err
		}
	}
	configMapData, err := entryPointData.Render()
	if err != nil {
		return false, err
	}

	newConfigMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-entrypoint", clusterOps.Name),
			Namespace: util.GetCurrentNSOrDefault(),
		},
		Data: map[string]string{"entrypoint.sh": strings.TrimSpace(configMapData)},
	}
	r.SetOwnerReferences(&newConfigMap.ObjectMeta, clusterOps)
	_, err = r.ClientSet.CoreV1().ConfigMaps(newConfigMap.Namespace).Create(context.Background(), newConfigMap, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		// exist and update
		klog.Warningf("entrypoint configmap %s already exist and update it.", newConfigMap.Name)
		if _, err := r.ClientSet.CoreV1().ConfigMaps(newConfigMap.Namespace).Update(context.Background(), newConfigMap, metav1.UpdateOptions{}); err != nil {
			return false, err
		}
	} else if err != nil {
		return false, err
	}
	clusterOps.Spec.EntrypointSHRef = &api.ConfigMapRef{
		NameSpace: newConfigMap.Namespace,
		Name:      newConfigMap.Name,
	}
	if err := r.Client.Update(context.Background(), clusterOps); err != nil {
		return false, err
	}
	return true, nil
}

func (r *ClusterOperationReconciler) CreateKubeSprayJob(clusterOps *kubeonkubev1alpha1.ClusterOperation) (bool, error) {
	if !clusterOps.Status.JobRef.IsEmpty() {
		return false, nil
	}
	jobName := r.GenerateJobName(clusterOps)
	namespace := clusterOps.Spec.HostsConfRef.NameSpace
	job, err := r.ClientSet.BatchV1().Jobs(namespace).Get(context.Background(), jobName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// the job doest not exist , and will create the job.
			sa, err := r.GetServiceAccountName(util.GetCurrentNSOrDefault(), ServiceAccount)
			if err != nil {
				return false, err
			}
			klog.Warningf("create job %s for kubeonkubeClusterOp %s", jobName, clusterOps.Name)
			job = r.NewKubesprayJob(clusterOps, sa)
			r.SetOwnerReferences(&job.ObjectMeta, clusterOps)
			job, err = r.ClientSet.BatchV1().Jobs(job.Namespace).Create(context.Background(), job, metav1.CreateOptions{})
			if err != nil {
				return false, err
			}
		} else {
			klog.Error(err)
			return false, err
		}
	}
	clusterOps.Status.JobRef = &api.JobRef{
		NameSpace: job.Namespace,
		Name:      job.Name,
	}
	clusterOps.Status.StartTime = &metav1.Time{Time: time.Now()}
	clusterOps.Status.Status = kubeonkubev1alpha1.RunningStatus
	clusterOps.Status.Action = clusterOps.Spec.Action

	if err := r.Client.Status().Update(context.Background(), clusterOps); err != nil {
		return false, err
	}
	return true, nil
}

func (r *ClusterOperationReconciler) GenerateJobName(clusterOps *kubeonkubev1alpha1.ClusterOperation) string {
	return fmt.Sprintf("kubeonkube-%s-job", clusterOps.Name)
}

// GetServiceAccountName get serviceaccount name on kubeonkube namespace by labelSelector.
func (r *ClusterOperationReconciler) GetServiceAccountName(namespace, labelSelector string) (string, error) {
	serviceAccounts, err := r.ClientSet.CoreV1().ServiceAccounts(namespace).List(context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return "", err
	}
	if len(serviceAccounts.Items) <= 0 {
		return "", fmt.Errorf("%s no valild serviceaccount", namespace)
	}
	return serviceAccounts.Items[0].Name, nil
}

func (r *ClusterOperationReconciler) NewKubesprayJob(clusterOps *kubeonkubev1alpha1.ClusterOperation, serviceAccountName string) *batchv1.Job {
	BackoffLimit := int32(0)
	DefaultMode := int32(0o700)
	PrivatekeyMode := int32(0o400)
	jobName := r.GenerateJobName(clusterOps)
	namespace := util.GetCurrentNSOrDefault()
	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      jobName,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &BackoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: serviceAccountName,
					Containers: []corev1.Container{
						{
							Name:    SprayJobPodName,
							Image:   clusterOps.Spec.Image,
							Command: []string{"/bin/entrypoint.sh"},
							Env: []corev1.EnvVar{
								{
									Name:  "CLUSTER_NAME",
									Value: clusterOps.Spec.Cluster,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "entrypoint",
									MountPath: "/bin/entrypoint.sh",
									SubPath:   "entrypoint.sh",
									ReadOnly:  true,
								},
								{
									Name:      "hosts-conf",
									MountPath: "/conf/hosts.yml",
									SubPath:   "hosts.yml",
								},
								{
									Name:      "vars-conf",
									MountPath: "/conf/group_vars.yml",
									SubPath:   "group_vars.yml",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "entrypoint",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: clusterOps.Spec.EntrypointSHRef.Name,
									},
									DefaultMode: &DefaultMode,
								},
							},
						},
						{
							Name: "hosts-conf",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: clusterOps.Spec.HostsConfRef.Name,
									},
								},
							},
						},
						{
							Name: "vars-conf",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: clusterOps.Spec.VarsConfRef.Name,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	if !clusterOps.Spec.SSHAuthRef.IsEmpty() {
		// mount ssh data
		if len(job.Spec.Template.Spec.Containers) > 0 && job.Spec.Template.Spec.Containers[0].Name == SprayJobPodName {
			job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts,
				corev1.VolumeMount{
					Name:      "ssh-auth",
					MountPath: "/auth/ssh-privatekey",
					SubPath:   "ssh-privatekey",
					ReadOnly:  true,
				})
		}
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: "ssh-auth",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  clusterOps.Spec.SSHAuthRef.Name,
						DefaultMode: &PrivatekeyMode, // fix Permissions 0644 are too open
					},
				},
			})
	}
	if clusterOps.Spec.ActiveDeadlineSeconds != nil && *clusterOps.Spec.ActiveDeadlineSeconds > 0 {
		job.Spec.ActiveDeadlineSeconds = clusterOps.Spec.ActiveDeadlineSeconds
	}
	if !reflect.ValueOf(clusterOps.Spec.Resources).IsZero() {
		if len(job.Spec.Template.Spec.Containers) > 0 && job.Spec.Template.Spec.Containers[0].Name == SprayJobPodName {
			job.Spec.Template.Spec.Containers[0].Resources = clusterOps.Spec.Resources
		}
	}
	return job
}

func (r *ClusterOperationReconciler) UpdateStatusLoop(clusterOps *kubeonkubev1alpha1.ClusterOperation, fetchJobStatus func(*kubeonkubev1alpha1.ClusterOperation) (kubeonkubev1alpha1.OpsStatus, *metav1.Time, error)) (bool, error) {
	if clusterOps.Status.Status == kubeonkubev1alpha1.RunningStatus || len(clusterOps.Status.Status) == 0 {
		// need fetch jobStatus again when the last status of job is running
		jobStatus, completionTime, err := fetchJobStatus(clusterOps)
		if err != nil {
			return false, err
		}
		if jobStatus == kubeonkubev1alpha1.RunningStatus {
			// still running
			return true, nil
		}
		// the status  succeed or failed
		clusterOps.Status.Status = jobStatus
		clusterOps.Status.EndTime = &metav1.Time{Time: time.Now()}
		if completionTime != nil {
			clusterOps.Status.EndTime = completionTime
		}
		if err := r.Client.Status().Update(context.Background(), clusterOps); err != nil {
			return false, err
		}
		return false, nil
	}
	// already finished(succeed or failed)
	return false, nil
}

func (r *ClusterOperationReconciler) FetchJobConditionStatusAndCompletionTime(clusterOps *kubeonkubev1alpha1.ClusterOperation) (kubeonkubev1alpha1.OpsStatus, *metav1.Time, error) {
	if clusterOps.Status.JobRef.IsEmpty() {
		return "", nil, fmt.Errorf("clusterOps %s no job", clusterOps.Name)
	}
	targetJob, err := r.ClientSet.BatchV1().Jobs(clusterOps.Status.JobRef.NameSpace).Get(context.Background(), clusterOps.Status.JobRef.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		// maybe the job is removed.
		klog.Errorf("clusterOps %s  job %s not found", clusterOps.Name, clusterOps.Status.JobRef.Name)
		return kubeonkubev1alpha1.FailedStatus, nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	// according to the job condtions, return success or failed
	for _, contion := range targetJob.Status.Conditions {
		if contion.Type == batchv1.JobComplete && contion.Status == corev1.ConditionTrue {
			return kubeonkubev1alpha1.SucceededStatus, targetJob.Status.CompletionTime, nil
		}
		if contion.Type == batchv1.JobFailed && contion.Status == corev1.ConditionTrue {
			return kubeonkubev1alpha1.FailedStatus, targetJob.Status.CompletionTime, nil
		}
		if contion.Type == batchv1.JobFailureTarget && contion.Status == corev1.ConditionTrue {
			return kubeonkubev1alpha1.FailedStatus, targetJob.Status.CompletionTime, nil
		}
		if contion.Type == batchv1.JobSuspended && contion.Status == corev1.ConditionTrue {
			return kubeonkubev1alpha1.FailedStatus, targetJob.Status.CompletionTime, nil
		}
	}

	return kubeonkubev1alpha1.RunningStatus, nil, nil
}

func (r *ClusterOperationReconciler) UpdateStatusForLabel(clusterOps *kubeonkubev1alpha1.ClusterOperation) error {
	if clusterOps.Labels == nil {
		clusterOps.Labels = make(map[string]string)
	}
	if clusterOps.Labels["hasCompleted"] == "done" {
		return nil
	}
	if clusterOps.Status.Status == kubeonkubev1alpha1.SucceededStatus || clusterOps.Status.Status == kubeonkubev1alpha1.FailedStatus {
		clusterOps.Labels["hasCompleted"] = "done"
		if err := r.Client.Update(context.Background(), clusterOps); err != nil {
			return err
		}
	}
	return nil
}
