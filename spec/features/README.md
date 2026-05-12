# Features

This directory will hold feature specifications for [DALgo](https://github.com/dal-go/dalgo) as they are written.

The Feature format follows [SpecScore](https://specscore.md/feature-specification).

## Index

| Feature | Status | Summary |
|---|---|---|
| [concurrency-capability](concurrency-capability/README.md) | Implemented | Add `dal.ConcurrencyAware` capability interface embedded in `dal.DB`, plus `NoConcurrency`/`ConcurrencyAvailable` embeddable helper structs, so consumers can size worker pools without engine-specific knowledge. |

## Outstanding Questions

None at this time.

---
*This document follows the https://specscore.md/features-index-specification*
