// This is a generated file. Do not edit directly.

module k8s.io/api

go 1.16

require (
	github.com/gogo/protobuf v1.3.2
	github.com/stretchr/testify v1.7.0
	k8s.io/apimachinery v0.0.0
)

replace (
	k8s.io/api => ../api
	k8s.io/apimachinery => ../apimachinery
	k8s.io/klog/v2 => github.com/pohly/klog/v2 v2.40.2-0.20220214120605-811b9422fa7d
)
