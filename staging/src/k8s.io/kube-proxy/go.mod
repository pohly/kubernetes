// This is a generated file. Do not edit directly.

module k8s.io/kube-proxy

go 1.16

require (
	k8s.io/apimachinery v0.0.0
	k8s.io/component-base v0.0.0
)

replace (
	k8s.io/api => ../api
	k8s.io/apimachinery => ../apimachinery
	k8s.io/client-go => ../client-go
	k8s.io/component-base => ../component-base
	k8s.io/klog/v2 => github.com/pohly/klog/v2 v2.40.2-0.20220214120605-811b9422fa7d
	k8s.io/kube-proxy => ../kube-proxy
)
