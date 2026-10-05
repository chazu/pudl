# pudl-cbv: Unit Tests for Collection Wrapper Detection

## Summary
Created `internal/importer/wrapper_test.go` with comprehensive unit tests for `DetectCollectionWrapper`.

## Test Coverage
- **8 positive cases**: simple wrapper, count match, pagination, envelope pattern, Stripe-like, AWS DynamoDB (case-insensitive), large homogeneous array, Elasticsearch hits
- **7 negative cases**: attribute key (tags), too many scalars, multiple similar arrays, primitive array, empty array, empty map, resource with many scalars
- **4 edge cases**: at threshold, below threshold, single-element with pagination, unknown key with strong signals
- **2 additional tests**: WrapperMeta extraction correctness, best-candidate-wins selection

## Public API Tested
- `DetectCollectionWrapper(data map[string]interface{}) *WrapperDetection`
- Verified `WrapperDetection` fields: `ArrayKey`, `Items`, `WrapperMeta`, `Score`, `Signals`
