---
title: Configure CI/CD
permalink: advanced/ci_cd/run_in_container/pull_model/configure_ci_cd.html
---

According to the [pull model overview](#TODO) we need to setup 


## Prepare release artifacts with werf

```
werf bundle publish --repo CONTAINER_REGISTRY --tag "0.1.${CI_PIPELINE_ID}"
```

## Setup continuous deployment of release artifacts

To setup continuous deployment of release artifacts, configure ArgoCD Application CRD with the following annotations:

```
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  annotations:
    argocd-image-updater.argoproj.io/chart-version: ~ 0.1
    argocd-image-updater.argoproj.io/pull-secret: pullsecret:NAMESPACE/regcred
```

The value of `argocd-image-updater.argoproj.io/chart-version="~ 0.1"` means that operator should automatically rollout chart to the latest patch version within semver range `0.1.*`.
