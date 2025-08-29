# Personal Unified Data Lake (PUDL)

PUDL is a CLI tool that helps SRE/platform engineers and software engineers manage and analyze cloud infrastructure data. It creates a personal data lake for cloud resources, Kubernetes objects, logs, and metrics with automatic schema detection and validation using CUE Lang.

## Key Features

- **Automatic Data Import**: Import JSON, YAML, and CSV files with intelligent format and schema detection
- **Schema Management**: CUE-based schemas with git version control and comprehensive validation
- **Data Discovery**: Powerful filtering, sorting, and search capabilities across all imported data
- **Metadata Tracking**: Complete provenance tracking with timestamps, origins, and schema assignments
- **Package Organization**: Organize schemas by source type (AWS, Kubernetes, custom, etc.)
- **Professional CLI**: Git-like command structure with comprehensive help and error handling

## Installation & Setup

### Prerequisites
- Go 1.19+ (for building from source)
- Git (for schema version control)

### Quick Start

```bash
# Build PUDL
go build -o pudl .

# Initialize your workspace (creates ~/.pudl/)
./pudl init

# You're ready to go!
./pudl --help
```

## Usage Guide

### 1. Workspace Management

```bash
# Initialize PUDL workspace
pudl init

# View configuration
pudl config

# Modify configuration
pudl config set data_path ~/my-pudl-data
pudl config reset  # Reset to defaults
```

### 2. Data Import

Import data from various formats with automatic schema detection:

```bash
# Import a single file
pudl import --path aws-ec2-instances.json
pudl import --path k8s-pods.yaml
pudl import --path metrics.csv

# Override origin detection
pudl import --path data.json --origin my-custom-source
```

**Supported Formats:**
- JSON (`.json`)
- YAML (`.yaml`, `.yml`)
- CSV (`.csv`)

**Automatic Features:**
- Format detection from file extension and content
- Origin inference from filename patterns (aws-ec2, k8s-pods, etc.)
- Schema assignment using rule-based detection
- Metadata tracking with timestamps and provenance

### 3. Data Discovery & Inspection

```bash
# List all imported data
pudl list

# Filter by schema type
pudl list --schema aws.#EC2Instance
pudl list --schema k8s.#Pod

# Filter by origin or format
pudl list --origin aws-ec2 --format json

# Sort and limit results
pudl list --sort-by size --reverse --limit 10

# Verbose output with file paths and statistics
pudl list --verbose

# Inspect specific data entry
pudl show 20250825_222545_aws-ec2-describe-instances
pudl show <entry-id> --metadata  # Show metadata
pudl show <entry-id> --raw       # Show raw data
```

### 4. Schema Management

PUDL uses CUE schemas organized by packages for data validation and structure definition.

```bash
# List all available schemas
pudl schema list

# List schemas in specific package
pudl schema list --package aws
pudl schema list --package k8s --verbose

# Add a new schema
pudl schema add aws.rds-instance my-rds-schema.cue
pudl schema add custom.api-response api-schema.cue
```

**Schema Naming Convention:**
- Format: `package.name` (e.g., `aws.ec2-instance`, `k8s.deployment`)
- Packages: `aws`, `k8s`, `custom`, `unknown`
- Files stored in: `~/.pudl/schema/package/name.cue`

**Schema Requirements:**
- Valid CUE syntax
- Package declaration matching target package
- Recommended metadata fields: `_identity`, `_tracked`, `_version`

### 5. CUE Processing

Process CUE files with custom functions (legacy feature):

```bash
# Process a CUE file
pudl process example.cue
```

## Current Implementation Status

**✅ Phase 1: CLI Foundation** (Complete)
- Professional Cobra CLI structure
- Workspace initialization and configuration management
- Auto-initialization for seamless user experience

**✅ Phase 2: Data Storage Foundation** (Complete)
- Data import with automatic format/schema detection
- Date-partitioned storage with metadata tracking
- Data discovery with filtering, sorting, and inspection

**✅ Phase 3.1: Schema Storage** (Complete)
- Schema management with package organization
- CUE validation with comprehensive error checking
- Git-based version control for schema repository

**🔄 Next: Phase 3.2** - Schema-Data Association
- Manual schema assignment during import
- Data validation against schemas
- Enhanced import workflow with schema selection

## Data Organization

PUDL organizes your data in a structured workspace:

```
~/.pudl/
├── config.yaml              # PUDL configuration
├── schema/                   # Git repository for schemas
│   ├── aws/                 # AWS resource schemas
│   │   ├── ec2.cue         # EC2 instance schema
│   │   └── rds-instance.cue # RDS instance schema
│   ├── k8s/                 # Kubernetes schemas
│   │   └── resources.cue    # Pod, Service, etc.
│   ├── custom/              # Custom schemas
│   └── unknown/             # Catchall schemas
└── data/
    ├── raw/YYYY/MM/DD/      # Date-partitioned raw data
    ├── metadata/            # Import metadata files
    └── catalog/             # Data inventory and indexes
```

## Example Workflows

### Import and Analyze AWS Data
```bash
# Import EC2 instance data
pudl import --path aws-ec2-describe-instances.json

# List all AWS resources
pudl list --schema aws --verbose

# Inspect specific instance
pudl show 20250825_222545_aws-ec2-describe-instances --raw
```

### Manage Custom Schemas
```bash
# Create a custom schema file (my-api.cue)
# Add it to PUDL
pudl schema add custom.api-response my-api.cue

# Import data using the new schema
pudl import --path api-data.json

# Review the schema
pudl schema list --package custom --verbose
```

### Data Discovery
```bash
# Find all YAML files imported in the last week
pudl list --format yaml --sort-by timestamp --reverse

# Find large data files
pudl list --sort-by size --reverse --limit 5

# Get overview of all data
pudl list --verbose
```

For detailed implementation progress and roadmap, see `plan.md`.
