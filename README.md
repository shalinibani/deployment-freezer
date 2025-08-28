# deployment-freezer
`deployment-freezer` temporarily scales down a deployment's replica count to 0 for the specified `freezeDurationInSec` and restores it afterward.

## Description
This project is scaffolded using `kubebuilder` using the following commands to initialize the project and generate a custom operator:
```
kubebuilder init --domain=mydomain.dev --repo=github.com/shalinibani/deployment-freezer
```
```
kubebuilder create api --group=apps --version=v1 --kind=DeploymentFreezer --resource --controller
```

## Getting Started

### Prerequisites
- go version v1.24.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a minikube cluster for local testing and run.

First, install minikube from [https://minikube.sigs.k8s.io](https://minikube.sigs.k8s.io/docs/start/?arch=%2Fmacos%2Fx86-64%2Fstable%2Fbinary+download).

Next, run `minikube start` to launch a cluster on your machine.
After the cluster starts running, follow the following steps:

**Install the CRDs into the cluster:**

```sh
make install
```

You can now apply the CR from the config/sample:

```sh
kubectl apply -k config/samples/
```

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

