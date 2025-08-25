# Personal Unified Data Lake

PUDL is a golang CLI tool which aims to help those who work with cloud resources amplify their ability to leverage data as part of their regular workflows. It manages the import, querying, processing and updating of a local 'data lake' comprising data on remote resources such as AWS or GCP resources, Kubernetes resources, logs, metrics, et cetera. It operates according to a few basic principles

* Schema are managed using cuelang. All schema are kept under version control in a git repository, stored by default at ~/.pudl/schema/

* Users can create schema by hand from scratch, or use cuelang's import abilities to import schema from any data source the cue cli is capable of importing. They can also have the pudl cli scaffold a new schema based on an input datum. When the user asks pudl to create a new schema from sample data, pudl executes a rule engine to determine how to model the schema

* All business rules in the pudl system are written in zygomys, a lisp which can be embedded in golang. This includes the rule engine used to infer and generate schema from sample data and other business logic where it is advantageous to provide extensibility.

* Users can import data by piping it into the pudl cli, or by using `pudl import --import-path <path>` where path is a file, directory or glob. On importing, if a schema is not specified with the `--schema` flag, and if the user does not deactivate schema inferrence via `--disable-schema-inferrence`, pudl will first determine the format of the data (json, yaml, csv, xml, plain string, etc) and deserailize it if necessary, then attempt to infer the schema of the data being imported by applying a series of production rules and checking the data against a registry of all schema in the schema repository. For example, it will determine whether the data, if it was deserialized, is a collection of items or an item, and use this fact to narrow down the possible schema. To reiterate, these production rules are written in zygmoys so as to provide a simple, uniform, powerful, maintainble means of extending functionality.
