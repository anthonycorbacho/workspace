## Overlays

This folder contains all the Kustomize deployment scope.

Each scope (local, staging, production) will have a different overlay setting flavor depending on what is going to be included.

for example, staging and production might runs on different cloud provider and will mostly have different service-accounts, configs.