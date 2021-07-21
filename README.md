# Spacectx

> Easy and flexible context management for Spacelift CI/CD

Spacectx offers another way to manage complex contexts in Spacelift CI/CD. Contexts are a great way to share data between stacks instead of using remote data sources. It solves the problem of having to grant access to the state storage across environments and offers better control on which stacks are accessing which context.

However, only supporting environment variables and mounted files it can be a bit complicated to replace remote data sources completely as they can store much more complex data structure. In addition, if a stack wants to create a spacelift context it needs to add those resources to the terraform configuration.

Spacectx tries to solve some of these problems.

- It mirrors the same outputs as defined in terraform module into the context (as special mounted files). No need to create spacelift resources in the terraform files
- It can replace context variables in tfvars files to 
