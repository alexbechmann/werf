---
title: Pull model overview
permalink: advanced/ci_cd/run_in_container/pull_model/overview.html
---

In this article we will describe how to implement continuous delivery and continuous deployment for your project using pull model. It implies:
* usage of **werf** to build and publish release artifacts;
* usage of **argocd gitops tool** to deliver release artifacts into the kubernetes;
* gluing together werf and argocd with **bundles**.

## Pull model and CI/CD

We are aiming to implement CI/CD process with the following characteristics:

* There is a single source of truth — project's git repository, which contains:
  * application source code;
  * instructions to build images;
  * kubernetes manifests.
* Reproducible release artifacts.
  * Each commit 
* Project released into production early and often from 

## Gitops role in CI/CD

Gitops approach 

## Pull model in CI/CD

Gitops
