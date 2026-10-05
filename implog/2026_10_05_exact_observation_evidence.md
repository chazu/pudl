# Exact observation evidence

Observation JSON now uses json.Decoder.UseNumber and rejects additional JSON values before catalog writes. Regression coverage checks stored adjacent large integers, nested fractional/exponent values, and atomic rejection of trailing input. Manifest ingestion uses typed integer/string fields and json.Unmarshal already rejects trailing data; no lossy any-number boundary exists there. Envelope data remains json.RawMessage.
