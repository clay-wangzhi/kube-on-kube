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

### 新增 clientset,informer,lister
```bash
# 1. 新增 hack/tools.go 文件，安装依赖包，参考 https://github.com/kubernetes/sample-controller/blob/master/hack/tools.go
go get k8s.io/code-generator@v0.26.1
go mod vendor
chmod +x vendor/k8s.io/code-generator/generate-groups.sh
# 2. 新增hack/update-codegen.sh，参考 https://github.com/kubernetes/sample-controller/blob/master/hack/update-codegen.sh
注意修改几个变量：

MODULE和go.mod保持一致
API_PKG=apis，和apis目录保持一致
OUTPUT_PKG=generated/webapp，生成Resource时指定的group一样
GROUP_VERSION=webapp:v1和生成Resource时指定的group version对应

# 3. 新增 hack/verify-codegen.sh , 参考 https://github.com/kubernetes/sample-controller/blob/master/hack/verify-codegen.sh

# 4. 改type
添加上tag // +genclient
新增 doc.go
新增 register.go

 chmod +x  ./hack/update-codegen.sh
 ./hack/update-codegen.sh
```



