# Human-Friendly ID System

PUDL now supports human-friendly IDs that are shorter, more memorable, and easier to communicate than the previous timestamp-based format.

## 🎯 ID Format Options

| Format                  | Example           | Length   | Best For                              | Pronounceable |
| ----------------------- | ----------------- | -------- | ------------------------------------- | ------------- |
| **Proquint** ⭐         | `sohiv-vogur`     | 11 chars | **General use, verbal communication** | ✅ Yes        |
| **Proquint (Prefixed)** | `col-loruk-pudon` | 15 chars | **Collections, team communication**   | ✅ Yes        |
| **Compact**             | `100725-ygw`      | 10 chars | **Date-aware data**                   | ❌ No         |
| **Compact (Prefixed)**  | `k8s-100725-3gx`  | 14 chars | **AWS/K8s with date context**         | ❌ No         |
| **Short Code**          | `2mq322`          | 6 chars  | **APIs, compact storage**             | ❌ No         |
| **Readable**            | `bright-cat-32`   | 13 chars | **Human memory**                      | ✅ Partially  |
| **Sequential**          | `data-001`        | 8 chars  | **Ordered datasets**                  | ✅ Yes        |

## 🌟 Recommended: Proquints

**Proquints** are the recommended default format because they:

- **Pronounceable**: Easy to say over phone/video calls ("sohiv vogur")
- **Memorable**: Follow consonant-vowel patterns humans remember well
- **Compact**: Only 11 characters (74% shorter than legacy)
- **Unique**: Based on 32-bit numbers, very low collision probability
- **Standardized**: Based on published specification

### Proquint Examples

```
sohiv-vogur    → "SO-hiv VO-gur"
loruk-pudon    → "LO-ruk PU-don"
lusab-babad    → "LU-sab BA-bad"
```

## 📅 Compact Format with MMDDYY

For **AWS and Kubernetes data**, the compact format includes date context:

### Format Breakdown: `k8s-100725-3gx`

- **`k8s`** - Context prefix (Kubernetes)
- **`100725`** - Date in MMDDYY format (October 7th, 2025)
- **`3gx`** - Random 3-character uniqueness suffix

### Date Examples

- **January 15, 2025**: `aws-011525-abc`
- **December 25, 2024**: `k8s-122524-xyz`
- **February 3, 2026**: `aws-020326-def`

## 🧠 Context-Aware Selection

The system automatically chooses appropriate formats based on data origin:

```go
// AWS data
"aws-ec2-describe-instances" → "aws-100725-ps6"

// Kubernetes data
"kubectl-get-pods" → "k8s-100725-j18"

// Collections
"collection-import" → "col-poqun-jokit"

// General data
"unknown-data-source" → "sofuk-fokis"
```

## 📦 Collection Item Handling

For NDJSON collections, the system intelligently handles item IDs:

### Data-Driven IDs

```json
{ "id": "user-123", "name": "John" }
```

→ Collection item ID: `col-lusin-kumob-user-123`

### Fallback IDs

```json
{ "name": "Jane", "email": "jane@example.com" }
```

→ Collection item ID: `col-lusin-kumob-jane-smith` (extracted from name)

### Index-Based IDs

```json
{ "data": "some data without clear identifier" }
```

→ Collection item ID: `col-lusin-kumob-item-00`

## 🎯 Format Benefits

**Compact and human-friendly** compared to traditional timestamp-based IDs:

| Traditional Timestamp ID                                | Human-Friendly ID           | Improvement     |
| ------------------------------------------------------- | --------------------------- | --------------- |
| `20241207_143052_aws-ec2-describe-instances` (42 chars) | `aws-100725-ps6` (14 chars) | **67% shorter** |
| `20241207_143053_kubectl-get-pods` (32 chars)           | `k8s-100725-j18` (14 chars) | **56% shorter** |
| `20241207_143054_unknown-data` (28 chars)               | `sofuk-fokis` (11 chars)    | **61% shorter** |

## ⚙️ Configuration

### Default Configuration

```json
{
  "default_format": "proquint",
  "enable_friendly_ids": true,
  "legacy_compatibility": true,
  "origin_overrides": {
    "aws": { "format": "compact", "prefix": "aws" },
    "kubernetes": { "format": "compact", "prefix": "k8s" },
    "collection": { "format": "proquint", "prefix": "col" }
  }
}
```

### Customization Examples

#### Use proquints for everything:

```go
config := &IDGenerationConfig{
    DefaultFormat: idgen.FormatProquint,
    EnableFriendlyIDs: true,
}
```

#### Use compact format for all data:

```go
config := &IDGenerationConfig{
    DefaultFormat: idgen.FormatCompact,
    GlobalPrefix: "data",
}
```

#### Custom origin mappings:

```go
config.OriginOverrides["my-service"] = idgen.IDConfig{
    Format: idgen.FormatReadable,
    Prefix: "svc",
}
```

## 🎨 Display Features

### Automatic Type Detection

The system automatically identifies ID types for display:

```go
displayHelper := idgen.NewIDDisplayHelper()

displayHelper.GetIDType("sohiv-vogur")     // → "proquint"
displayHelper.GetIDType("k8s-100725-3gx") // → "compact"
displayHelper.GetIDType("bright-cat-32")  // → "readable"
```

## 🚀 Usage Examples

### Basic ID Generation

```go
// Create proquint generator
gen := idgen.NewIDGenerator(idgen.FormatProquint, "")
id := gen.Generate() // → "sohiv-vogur"

// Create AWS compact generator
awsGen := idgen.NewIDGenerator(idgen.FormatCompact, "aws")
awsID := awsGen.Generate() // → "aws-100725-3gx"
```

### Importer Integration

```go
// Create enhanced importer with friendly IDs
importer, err := NewEnhancedImporter(dataPath, schemaPath, configDir, catalogDB)

// Import with automatic ID generation
result, err := importer.ImportFileWithFriendlyIDs(ImportOptions{
    SourcePath: "/data/aws-instances.json",
    Origin:     "aws-ec2-describe-instances",
})

// Result ID will be: "aws-100725-abc" (compact format for AWS)
```

### Collection Items

```go
manager := idgen.NewImporterIDManagerFromOrigin("collection-import")

// Generate collection ID
collectionID := manager.GenerateCollectionID("/data/users.ndjson", "user-collection")
// → "col-sohiv-vogur"

// Generate item IDs
itemID := manager.GenerateItemID(collectionID, 0, map[string]interface{}{
    "id": "user-123",
    "name": "John Doe",
})
// → "col-sohiv-vogur-user-123"
```

## 🔧 Implementation Status

✅ **Core ID Generation** - All formats implemented and tested  
✅ **Context-Aware Selection** - Automatic format selection by origin  
✅ **Collection Support** - Smart item ID generation  
✅ **Smart ID Generation** - Context-aware format selection
✅ **Display Helpers** - UI-friendly formatting  
✅ **Configuration System** - Flexible format customization  
✅ **Comprehensive Tests** - Full test coverage including proquint conversion  
✅ **MMDDYY Date Format** - Enhanced compact format with year

## 📝 Next Steps

1. **UI Updates** - Display friendly IDs in web interface and CLI
2. **Enhanced Configuration** - Additional customization options
3. **Documentation** - Update user guides and API documentation

The human-friendly ID system is ready for production use and provides a significant improvement in usability with compact, memorable, and pronounceable identifiers.
