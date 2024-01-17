The files in this directory were created with:

    controller-gen crd paths=./test/e2e/dra/test-driver/api/... output:dir=test/e2e/dra/test-driver/deploy/crd

There's no update+verify script for these CRDs because that would imply that
k/k has to depend on controller-gen, which itself depends on staging repos. To
minimize the amount of generated code, the tests and test driver use a dynamic
client to handle these CRDs.
