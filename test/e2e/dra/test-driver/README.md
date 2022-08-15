# dra-test-driver

This driver implements the controller and a resource kubelet plugin for dynamic
resource allocation. This is done in a single binary to minimize the amount of
boilerplate code. "Real" drivers could also implement both in different
binaries.

## Usage

The driver could get deployed as a Deployment for the controller, with leader
election. A DaemonSet could get used for the kubelet plugin. The controller can
also run as a Kubernetes client outside of a cluster. The same works for the
kubelet plugin when using port forwarding. This is how it is used during
testing.

The actual functionality is initially very limited. Resources are unlimited, so
all ResourceClaims can be allocated. Valid parameters are key/value string
pairs stored in a ConfigMap.

Those get copied into the ResourceClaimStatus with "user_" and "admin_" as
prefix, depending on whether they came from the ResourceClaim or ResourceClass.
They get stored in the `ResourceHandle` field as JSON map by the controller.
The kubelet plugin then sets these attributes as environment variables in each
container that uses the resource.

While the functionality itself is very limited, the code strives to showcase
best practices and supports metrics, leader election, and the same logging
options as Kubernetes.

## Design

The binary itself is a Cobra command with two operations, `controller` and
`kubelet-plugin`. Logging is done with [contextual
logging](https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/3077-contextual-logging).

The `k8s.io/component-helpers/dra/controller` package implements the
interaction with ResourceClaims. It is generic and relies on an interface to
implement the actual driver logic. Long-term that part could be split out into
a reusable utility package.

The `k8s.io/component-helpers/dra/kubelet-plugin` package implements the
interaction with kubelet, again relying only on the interface defined for the
kubelet<->dynamic resource allocation plugin interaction.

`app` is the driver itself with a very simple implementation of the interfaces.

## Prior art

Some of this code was derived from the
[external-resizer](https://github.com/kubernetes-csi/external-resizer/). `controller`
corresponds to the [controller
logic](https://github.com/kubernetes-csi/external-resizer/blob/master/pkg/controller/controller.go),
which in turn is similar to the
[sig-storage-lib-external-provisioner](https://github.com/kubernetes-sigs/sig-storage-lib-external-provisioner).
