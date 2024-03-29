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
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"kube-on-kube/pkg/util"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubeonkubev1alpha1 "kube-on-kube/api/kubeonkube/v1alpha1"
	kokClientSet "kube-on-kube/generated/clientset/versioned"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klog "k8s.io/klog/v2"
)

const (
	RequeueAfter                         = time.Second * 15
	KubeonkubeConfigMapName              = "kubeonkube-config"
	DefaultClusterOperationsBackEndLimit = 30
	MaxClusterOperationsBackEndLimit     = 200
	EliminateScoreAnno                   = "clay.io/eliminate-score"
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	Client       client.Client
	Scheme       *runtime.Scheme
	ClientSet    kubernetes.Interface
	KokClientSet kokClientSet.Interface
}

//+kubebuilder:rbac:groups=kubeonkube.clay.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubeonkube.clay.io,resources=clusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubeonkube.clay.io,resources=clusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Cluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	// 判断 cluster 是否存在
	cluster := &kubeonkubev1alpha1.Cluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		klog.ErrorS(err, "failed to get cluster", "cluster", req.String())
		return ctrl.Result{RequeueAfter: RequeueAfter}, nil
	}

	// 判断是否需要备份清理
	OpsBackupNum := r.FetchKubeonkubeConfigProperty().GetClusterOperationsBackEndLimit()
	// 清理多余的 ClusterOps
	needRequeue, err := r.CleanExcessClusterOps(cluster, OpsBackupNum)
	if err != nil {
		klog.ErrorS(err, "failed to clean excess cluster ops", "cluster", cluster.Name)
		return ctrl.Result{RequeueAfter: RequeueAfter}, nil
	}
	// 在删除多余 ClusterOps ing，延迟加入队列，继续调谐
	if needRequeue {
		return ctrl.Result{RequeueAfter: RequeueAfter}, nil
	}

	// 更新状态
	if err := r.UpdateStatus(cluster); err != nil {
		klog.ErrorS(err, "failed to update cluster status", "cluster", cluster.Name)
		return ctrl.Result{RequeueAfter: RequeueAfter}, nil
	}

	// 更新 configData 和 secretData 的 OwnReference
	if err := r.UpdateOwnReferenceToCluster(cluster); err != nil {
		klog.ErrorS(err, "failed to update the ownReference configData or secretData", "cluster", cluster.Name)
		return ctrl.Result{RequeueAfter: RequeueAfter}, nil
	}

	// loop ，循环监听，当有新的 ClusterOps 任务进来后， 处理删除旧 ClusterOps 任务
	return ctrl.Result{RequeueAfter: RequeueAfter}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubeonkubev1alpha1.Cluster{}).
		Complete(r)
}

// 定义配置属性结构体
type ConfigProperty struct {
	ClusterOperationsBackEndLimit string `json:"CLUSTER_OPERATIONS_BACKEND_LIMIT"`
}

// 获取 kubeonkube 配置文件
func (r *ClusterReconciler) FetchKubeonkubeConfigProperty() *ConfigProperty {
	configData, err := r.ClientSet.CoreV1().ConfigMaps(util.GetCurrentNSOrDefault()).Get(context.Background(), KubeonkubeConfigMapName, metav1.GetOptions{})
	if err != nil {
		return &ConfigProperty{}
	}
	jsonData, err := json.Marshal(configData.Data)
	if err != nil {
		return &ConfigProperty{}
	}
	result := &ConfigProperty{}
	err = json.Unmarshal(jsonData, result)
	if err != nil {
		return &ConfigProperty{}
	}
	return result
}

// 备份限制 校验
func (config *ConfigProperty) GetClusterOperationsBackEndLimit() int {
	value, _ := strconv.Atoi(config.ClusterOperationsBackEndLimit)
	if value <= 0 {
		klog.Warningf("GetClusterOperationsBackEndLimit and use default value %d", DefaultClusterOperationsBackEndLimit)
		return DefaultClusterOperationsBackEndLimit
	}
	if value >= MaxClusterOperationsBackEndLimit {
		klog.Warningf("GetClusterOperationsBackEndLimit and use max value %d", MaxClusterOperationsBackEndLimit)
		return MaxClusterOperationsBackEndLimit
	}
	return value
}

// CleanExcessClusterOps clean up excess ClusterOperation.
func (r *ClusterReconciler) CleanExcessClusterOps(cluster *kubeonkubev1alpha1.Cluster, OpsBackupNum int) (bool, error) {
	listOpt := metav1.ListOptions{LabelSelector: fmt.Sprintf("clusterName=%s", cluster.Name)}
	clusterOpsList, err := r.KokClientSet.KubeonkubeV1alpha1().ClusterOperations().List(context.Background(), listOpt)
	if err != nil {
		return false, err
	}
	if len(clusterOpsList.Items) <= OpsBackupNum {
		return false, nil
	}

	r.SortClusterOperationsByCreation(clusterOpsList.Items)

	excessClusterOpsList := clusterOpsList.Items[OpsBackupNum:]
	for _, item := range excessClusterOpsList {
		if item.Status.Status == kubeonkubev1alpha1.RunningStatus { // keep running job
			continue
		}
		klog.Warningf("Delete ClusterOperation: name: %s, createTime: %s, status: %s", item.Name, item.CreationTimestamp.String(), item.Status.Status)
		r.KokClientSet.KubeonkubeV1alpha1().ClusterOperations().Delete(context.Background(), item.Name, metav1.DeleteOptions{})
	}
	return true, nil
}

// SortClusterOperationsByCreation sort operations order by EliminateScore ascend , createTime desc.
func (r *ClusterReconciler) SortClusterOperationsByCreation(operations []kubeonkubev1alpha1.ClusterOperation) {
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].CreationTimestamp.After(operations[j].CreationTimestamp.Time)
	})
	sort.Slice(operations, func(i, j int) bool {
		return r.GetEliminateScoreValue(operations[i]) < r.GetEliminateScoreValue(operations[j])
	})
}

// 获取排除分值
func (r *ClusterReconciler) GetEliminateScoreValue(operation kubeonkubev1alpha1.ClusterOperation) int {
	value, err := strconv.Atoi(operation.Annotations[EliminateScoreAnno])
	if err != nil {
		return 0
	}
	return value
}

func (r *ClusterReconciler) UpdateStatus(cluster *kubeonkubev1alpha1.Cluster) error {
	listOpt := metav1.ListOptions{LabelSelector: fmt.Sprintf("clusterName=%s", cluster.Name)}
	clusterOpslist, err := r.KokClientSet.KubeonkubeV1alpha1().ClusterOperations().List(context.Background(), listOpt)
	if err != nil {
		return err
	}
	// clusterOps list sort by creation timestamp
	r.SortClusterOperationsByCreation(clusterOpslist.Items)
	newConditions := make([]kubeonkubev1alpha1.ClusterCondition, 0)
	for _, item := range clusterOpslist.Items {
		newConditions = append(newConditions, kubeonkubev1alpha1.ClusterCondition{
			ClusterOps: item.Name,
			Status:     kubeonkubev1alpha1.ClusterConditionType(item.Status.Status),
			StartTime:  item.Status.StartTime,
			EndTime:    item.Status.EndTime,
		})
	}
	if !CompareClusterConditions(cluster.Status.Conditions, newConditions) {
		// 不一样，就更新
		cluster.Status.Conditions = newConditions
		klog.Warningf("update cluster %s status.condition", cluster.Name)
		return r.Client.Status().Update(context.Background(), cluster)
	}
	return nil
}

// 比较集群状态
func CompareClusterConditions(condAList, condBlist []kubeonkubev1alpha1.ClusterCondition) bool {
	if len(condAList) != len(condBlist) {
		return false
	}
	for i := range condAList {
		if !CompareClusterCondition(condAList[i], condBlist[i]) {
			return false
		}
	}
	return true
}

// 比较集群中每个 ClusterOps 的状态是否相同
func CompareClusterCondition(conditionA, conditionB kubeonkubev1alpha1.ClusterCondition) bool {
	unixMilli := func(t *metav1.Time) int64 {
		if t == nil {
			return -1
		}
		return t.UnixMilli()
	}
	if conditionA.ClusterOps != conditionB.ClusterOps {
		return false
	}
	if conditionA.Status != conditionB.Status {
		return false
	}
	if unixMilli(conditionA.StartTime) != unixMilli(conditionB.StartTime) {
		return false
	}
	if unixMilli(conditionA.EndTime) != unixMilli(conditionB.EndTime) {
		return false
	}
	return true
}

func (r *ClusterReconciler) UpdateOwnReferenceToCluster(cluster *kubeonkubev1alpha1.Cluster) error {
	return util.UpdateOwnReference(r.ClientSet, cluster.Spec.ConfigDataList(), cluster.Spec.SecretDataList(), *metav1.NewControllerRef(cluster, kubeonkubev1alpha1.SchemeGroupVersion.WithKind("Cluster")))
}
