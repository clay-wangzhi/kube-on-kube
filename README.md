### 环境说明
```bash
kubebuilder 3.10.0
go 1.20.3
```

### 初始化
```bash
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
```

### 改配置
```bash
Makefile
ENVTEST_K8S_VERSION = 1.18.10
make manifests
go mod vendor
```

### 改 type
```bash
# 改完
make
```
