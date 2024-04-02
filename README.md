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


### 编写调谐代码，改完，make install, 安装 crd
```bash
# 需要再本地配置 kubeconfig 
make install 
# 实际执行的是, 可以导出 crd 文件，在其他集群上 apply
kustomize build config/crd | kubectl apply -f -
```

### make run 临时测试，安装 controller
```bash
make run
```


### 在其他集群安装
```bash
# 生成 crd 
bin/kustomize build config/crd  > deploy/crd.yaml
# 生成 rbac, rbac 中roles 和 rolebing 没有指定 ns，需要修改, secrect、cm、job 权限需要添加
bin/kustomize build config/rbac > deploy/rbac.yaml
# 生成 deployment
bin/kustomize build config/manager > deploy/deployment.yaml
# 部署
kubectl apply -f crd.yaml
kubectl apply -f deployment.yaml
kubectl apply -f rbac.yaml
```


### 测试，安装 cluster yaml 和 clusterops yaml
```bash
kubectl -n kubeonkube   create secret generic sample-ssh-auth  --type='kubernetes.io/ssh-auth'   --from-file=ssh-privatekey=/home/clay/.ssh/id_rsa   --dry-run=client -o yaml > SSHAuthSec.yml  
```

准备 Host.yaml
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: sample-hosts-conf
  namespace: kubeonkube
data:
  hosts.yml: |
    all:
      hosts:
        master01:
          ip: 10.100.140.163
          access_ip: 10.100.140.163
          ansible_host: 10.100.140.163
          ansible_user: root
        worker01:
          ip: 10.100.140.169
          access_ip: 10.100.140.169
          ansible_host: 10.100.140.169
          ansible_user: root
      children:
        kube_control_plane:
          hosts:
            master01:
        kube_node:
          hosts:
            worker01:
```

准备 VarsConfCM.yml


准备 Cluster.yml
```yaml
apiVersion: kubeonkube.clay.io/v1alpha1
kind: Cluster
metadata:
  name: sample
  namespace: kubeonkube
spec:
  hostsConfRef:
    namespace: kubeonkube
    name: sample-hosts-conf
  varsConfRef:
    namespace: kubeonkube
    name: sample-vars-conf
  sshAuthRef: # 关键属性，指定集群部署期间的 ssh 私钥 secret
    namespace: kubeonkube
    name: sample-ssh-auth
```

准备 ClusterOperation.yml
```yaml
apiVersion: kubeonkube.clay.io/v1alpha1
kind: ClusterOperation
metadata:
  name: sample-create-cluster
  namespace: kubeonkube
spec:
  cluster: sample
  image: wangzhichidocker/kubeonkube:v0.1
  actionType: playbook
  action: precheck.yml
```