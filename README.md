### 环境说明
kubebuilder 3.10.0
go 1.20.3

### 初始化
kubebuilder init --domain clay.io --owner Clay --repo kube-on-kube
kubebuilder edit --multigroup=true
kubebuilder create api --group kubeonkube --version v1alpha1 --kind Cluster
Create Resource [y/n]
y
Create Controller [y/n]
y
kubebuilder create api --group kubeonkube --version v1alpha1 --kind ClusterOperation
Create Resource [y/n]
y
Create Controller [y/n]
y

### 改配置
Makefile
ENVTEST_K8S_VERSION = 1.18.10
make manifests
go mod vendor