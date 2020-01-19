# ValidatingWebhook for distributedredisclusters

## Prerequisites

Kubernetes with the `admissionregistration.k8s.io/v1beta1` API enabled. Verify that by the following command:
```
kubectl api-versions | grep admissionregistration.k8s.io/v1beta1
```
The result should be:
```
admissionregistration.k8s.io/v1beta1
```

In addition, the `MutatingAdmissionWebhook` and `ValidatingAdmissionWebhook` admission controllers should be added and listed in the correct order in the admission-control flag of kube-apiserver.

## Deploy

1. Create a signed cert/key pair and store it in a Kubernetes `secret` that will be consumed by operator deployment
```
./create-signed-cert.sh --service drc-admission-webhook --secret drc-webhook-cert --namespace default
```

2. Patch the `ValidatingWebhookConfiguration` by set `caBundle` with correct value from Kubernetes cluster
```
cat validatingwebhook.yaml | \
    patch-ca-bundle.sh > \
    validatingwebhook-ca-bundle.yaml
```

3. Deploy resources
```
kubectl delete -f operator.yml
kubectl create -f operator.yml
kubectl create -f validatingwebhook-ca-bundle.yaml
kubectl create -f service.yaml
```