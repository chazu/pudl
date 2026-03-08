# Writing PUDL Methods

Methods are Glojure (.clj) files that implement model operations. They live in `methods/<model-name>/<method-name>.clj`.

## Method Structure

Every method file must define a `run` function that takes an `args` map:

```clojure
(defn run [args]
  ;; args contains definition socket values, method inputs, and tags
  (let [instance-type (get args "InstanceType")
        image-id (get args "ImageId")]
    ;; Perform the operation and return a result
    {"InstanceId" "i-12345"
     "InstanceType" instance-type
     "ImageId" image-id
     "State" {"Name" "pending" "Code" 0}}))
```

## Method Kinds

### Action (default)
CRUD operations. Execute the core operation and return the result.

### Qualification
Precondition checks that run before actions. Must return `{:passed bool :message string}`:

```clojure
(defn run [args]
  (let [creds-valid (pudl.http/get-status "https://sts.amazonaws.com")]
    {"passed" (= creds-valid 200)
     "message" (if (= creds-valid 200)
                 "AWS credentials valid"
                 "AWS credentials invalid or expired")}))
```

### Attribute
Computed value derivation. Runs after the action to derive additional values.

### Codegen
Output transformation. Converts results to other formats (JSON, YAML, HCL).

## Available Builtins

Methods can call Go-registered functions in these namespaces:

### pudl.core
- `(pudl.core/upper s)` — Uppercase string
- `(pudl.core/lower s)` — Lowercase string
- `(pudl.core/trim s)` — Trim whitespace
- `(pudl.core/format fmt & args)` — String formatting
- `(pudl.core/env var)` — Read environment variable
- `(pudl.core/timestamp)` — Current ISO 8601 timestamp
- `(pudl.core/uuid)` — Generate UUID

### pudl.http
- `(pudl.http/get url)` — HTTP GET, returns body
- `(pudl.http/post url body)` — HTTP POST, returns body
- `(pudl.http/get-status url)` — HTTP GET, returns status code
- `(pudl.http/get-json url)` — HTTP GET, parses JSON response

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

With `--dry-run`, effects are listed but not executed, providing an audit trail.

## File Convention

```
methods/
  ec2_instance/
    list.clj
    create.clj
    delete.clj
    valid_credentials.clj
    ami_exists.clj
  simple/
    get.clj
```

The directory name must match the model's `metadata.name` field.
