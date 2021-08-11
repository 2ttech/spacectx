# Spacectx

> Easy and flexible context management for Spacelift CI/CD

Spacectx offers another way to manage complex contexts in Spacelift CI/CD. Contexts in Spacelift is a bundle of configuration elements, either environment variables or mounted files. It offers a different way to share configuration between stacks. Spacectx will auto generate the required context based on stack outputs and process `tfvars` files for context references.

## Problem

Sharing output / configuration between stacks can be done either by using terraform remote state or creating Spacelift contexts.

### Terraform remote state

- Requires access to the remote state. It is not possible to only grant access to the outputs from a stack
- Problematic for local development as it also required access to remote state for developer

### Spacelift context

- Requires adding spacelift resources to the module, which is problematic for local development
- Difficult to shared complex configuration since only supports environment variables and mounted files

## Solution

Spacectx tries to address these issues by using the best of Spacelift contexts. It will auto generate the requires resources required for creating a context during run, and add all outputs from stack to context (as a mounted file). That way there is no need to add any spacelift resources to the module so it is easy to still do local development and testing.

In addition it can also preprocess `tfvars` files and replace references to context variables. Typical way to use different config is to use a `before_init` hook:

```bash
mv terraform.workspace.tfvars terraform.auto.tfvars
```

This can instead be replaced with

```bash
spacectx process terraform.workspace.tfvars > terraform.auto.tfvars
```

### generate

```
spacectx generate ./directory
```

Generates the required spacelift resources mirroring the outputs defined in module directory. It will create 2 separate mounted files, one for regular outputs and one with secrets. Set input flag `--ignore-secrets` to skip creating the secrets file. This action has to be run on `before_init` hook, and stack has to be set to *administrative*.

### process

```
spacectx process terraform.workspace.tfvars
```

Process the `tfvars` file and replace references to context variables with actual value. NB! The context containing the variables need to be attached to the stack in Spacelift!

Stack `azure-virtual-network-dev` could defined following outputs:

```terraform
output "virtual_network_id" { value = "..." }
output "subnet_ids" { value = {
    "subnet1" = "...",
    "subnet2" = "...",
}}
```

By running `spacectx generate` in this stack it will generate a context called `azure-virtual-network-dev` that can be attached to current stack. Running `spacectx process terraform.workspace.tfvars` now makes it possible to reference this context using the `context` variable.

`terraform.workspace.tfvars`
```terraform
virtual_network_id = context.azure-virtual-network-dev.virtual_network_id
size = "BIGSIZE"
```
