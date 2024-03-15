## v1.0.1

Change from using a global random source to instead use the top level functions provided by `math/rand`. This only affects code paths using `DefaultDelayFn` or `CustomizedDelayFn`. Note that this package does not seed the top level random source, so it is left up to this package's consumer if that is desired.

## v1.0.0

Initial tagged version.

Renamed helper functions for setting context keys to be shorter by removing `OnContext` (for example, `SetShouldRetryFnOnContext` -> `SetShouldRetryFn`).