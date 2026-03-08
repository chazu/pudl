# Method Authoring Guide

Methods are Glojure (.clj) files that implement model operations. They provide the executable logic behind model method declarations.

## File Convention

Methods live in `methods/<model-name>/<method-name>.clj` within the schema directory:

```
~/.pudl/schema/methods/
  ec2_instance/
    list.clj
    create.clj
    delete.clj
    valid_credentials.clj
  simple/
    get.clj
```

The directory name must match the model's `metadata.name` field.

## Method Structure

Every method file must define a `run` function that takes an `args` map:

```clojure
(defn run [args]
  ;; args contains definition socket values, method inputs, and tags
  (let [instance-type (get args "InstanceType")
        image-id (get args "ImageId")]
    ;; Return a result map
    {"InstanceId" "i-12345"
     "InstanceType" instance-type
     "ImageId" image-id
     "State" {"Name" "pending" "Code" 0}}))
```

The `args` map is populated from:
- Definition socket bindings (with vault references resolved)
- Method input overrides from the definition
- Tags passed via `--tag` on the CLI

## Method Kinds

### Action (default)

CRUD and custom operations. Execute the core operation and return the result:

```clojure
(defn run [args]
  (let [url (get args "url")]
    (pudl.http/get-json url)))
```

### Qualification

Precondition checks that run before actions. Must return a map with `passed` (bool) and `message` (string):

```clojure
(defn run [args]
  (let [status (pudl.http/get-status "https://sts.amazonaws.com")]
    {"passed" (= status 200)
     "message" (if (= status 200)
                 "AWS credentials valid"
                 "AWS credentials invalid or expired")}))
```

Qualifications are declared on the model with `blocks: ["method1", "method2"]` to specify which methods they gate. When a method is invoked, all qualifications that block it run first. If any return `passed: false`, the method is aborted.

### Attribute

Computed value derivation. Runs after the action to derive additional values from the result.

### Codegen

Output transformation. Converts results to other formats (JSON, YAML, HCL).

## Available Builtins

Methods can call Go-registered functions in these namespaces:

### pudl.core

| Function | Description |
|----------|-------------|
| `(pudl.core/upper s)` | Uppercase string |
| `(pudl.core/lower s)` | Lowercase string |
| `(pudl.core/trim s)` | Trim whitespace |
| `(pudl.core/format fmt & args)` | String formatting |
| `(pudl.core/env var)` | Read environment variable |
| `(pudl.core/timestamp)` | Current ISO 8601 timestamp |
| `(pudl.core/uuid)` | Generate UUID |

### pudl.http

| Function | Description |
|----------|-------------|
| `(pudl.http/get url)` | HTTP GET, returns body string |
| `(pudl.http/post url body)` | HTTP POST, returns body string |
| `(pudl.http/get-status url)` | HTTP GET, returns status code |
| `(pudl.http/get-json url)` | HTTP GET, parses JSON response |

## Effect Pattern

Instead of executing side effects directly, methods can return an effect description for the runtime to process:

```clojure
(defn run [args]
  {"result" "planned"
   "pudl/effects" [{"kind" "create"
                     "description" "Launch EC2 instance"
                     "params" {"instance_type" (get args "InstanceType")
                               "image_id" (get args "ImageId")}}
                    {"kind" "http"
                     "description" "Notify deployment webhook"
                     "params" {"url" "https://hooks.example.com/deploy"
                               "method" "POST"}}]})
```

Effect kinds: `create`, `delete`, `update`, `http`, `exec`.

With `--dry-run`, effects are listed but not executed, providing an audit trail for review before committing changes.

## CLI Commands

```bash
# Execute a method
pudl method run prod_instance list

# Dry run — qualifications only, no action
pudl method run prod_instance create --dry-run

# Skip qualification checks
pudl method run prod_instance create --skip-advice

# Pass extra arguments
pudl method run prod_instance create --tag env=staging --tag region=us-east-1

# List available methods for a definition
pudl method list prod_instance
```

## Lifecycle Flow

```
pudl method run <definition> <method>
    |
    +-- Load definition, resolve args (socket values + inputs + tags)
    +-- Resolve vault:// references
    |
    +-- Run qualifications that block this method
    |     +-- If any fail -> abort with message
    |
    +-- Execute method .clj file -> call (run args)
    +-- Validate return value against CUE return schema
    |
    +-- Run post-actions (attribute, codegen methods)
    +-- Store result as immutable artifact in catalog
    +-- Update output socket values on definition
```
