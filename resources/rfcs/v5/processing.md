Processing
==========

When something fails during a processing pipeline there are many different patterns that a user may wish to follow, and it's entirely possible that multiple patterns may be required in a single pipeline:

### Deletion

On error the message is dropped and acknowledged, resulting in it being deleted from the stream. This is behavior that undermines at-least-once delivery guarantees, and therefore it is extremely important to ensure that this pattern is never followed without explicit and unambiguous instruction from a user.

### Rejection

On error the message is immediately rejected (nacked). This is often the behavior that users expect by default, and is most consistent with a strict at-least-once delivery guarantee when the processing steps are side effects or crucial to providing meaningful context to messages downstream.

### Recovery

On error the message is redirected through a dead letter queue whereby it is treated as a normal message again. This DLQ could be a specific output, or it could be a series of processors that aims to "recover" the message. This is a common practice in production environments where we need to keep the main feed clear of failed messages, but the messages themselves need to be routed somewhere in order to be asynchronously investigated.

### Enrichment

On error the message is enriched with a metadata value that describes the error, and then continues through the system in the same way that a regular message would. Other components (processors or outputs) are then able to detect the error and apply their own custom behavior to the message.

## Problem

When a processor fails the benthos engine by default follows the Enrichment pattern. This means that without explicit intervention via config the data will automatically continue through a pipeline unhindered upon the event of a processor error, ultimately being dispatched to the output.

Historically, this decision was made back when processing didn't have a formal error mechanism, we were instead simply adding actual metadata to the messages and it was left to users to decide how to treat the errors in their configs. This gave users the power to enforce any of the mentioned patterns via configuration but with a default behavior that makes debugging hard and could be interpretted to contradict our "at-least-once by default" claim.

We eventually expanded our processing error handling mechanisms such that we now have explicit errors associated with messages, multiple processors and output implementations (`try`, `catch`, `reject_errored`, on so on). However, the default was never changed.

## Solution

We should flip our default behavior to follow the Rejection pattern when processors fail. Then, in order to accommodate other patterns we add a new `recover` processor that wraps child processors with a recovery trigger:

```
pipeline:
  processors:
    - recover:
        processors:
          - cache:
              operator: set
              resource: foo
              key: ${! @key }
              value: ${! content() }
        on_error:
          - mapping: 'root.error = @recovered_error'
```

If any `processors` fail then instead of being rejected the `recover` mechanism moves the error into a metadata field on the message and then executes the processors within `on_error` on it.

If any of the processors within `on_error` fail the message will be rejected similar to a normal processor. This adds the unfortunate requirement of nested `recover` processors when the `on_error` logic is also capable of failure.

If a user has a processor that can fail, and they aren't interested in recovering at all, they simply want the message to continue, they can leave an empty `on_error` block (the default):

```
pipeline:
  processors:
    - recover:
        processors:
          - mapping: 'root = this.bar.parse_json()'
```

If this pattern becomes commonplace (unexpected) then we can always add a shorthand:

```
pipeline:
  processors:
    - ignore_error:
        mapping: 'root = this.bar.parse_json()'
```

### Logging

One common complaint about processor errors is that they are inconsistent in how they log. Some processors create error logs and others set them to the debug level. This distinction is normally due to a differing expectation as to whether it's "normal" for a processor to fail or not. For example, an http processor will always error log when a failure occurs, however, a `json_schema` processor does not as it's expected that when enforcing a schema (potentially many) messages will fail during normal operation.

This inconsistency exists purely as an attempt to avoid noisy logs from processors that fail continuously, and in order to solve this issue we need to enable users to customize logging themselves such that we can enable error level logs for all processors by default.

An already proposed solution to this is to add a `log_level_remap` processor that sets the logging level of child processors:

```
pipeline:
  processors:
    - log_level_remap:
        levels:
          error: debug
        processors:
          - mapping: 'root = this.bar.parse_json()'
```

This is a bit verbose, but in reality it is likely niche enough to be unnecessary in most use cases. We could even skip adding this processor until we feel pressure from the community to provide it.

## Implementation

### Step 1 (V4)

- We add a new `recover` processor.
- We deprecate the `try` and `catch` processors.
- We strongly encourage all users to migrate to `recover` and make common use of it (even if they are currently not using any error handling).
- We add a new log level processor that customizes the log level of underlying processors.
- We remove all custom logs from processor implementations and replace it with a catch-all log from the root implementation.
- This will be a soft introduction to the concept that processor errors are becoming "hard", with the final transition being made in v5.

#### Optional

- Implement hard rejection mode behind an environment variable (something like `XBENTHOS_HARD_FAILURE=1`). This is mostly for the purpose of giving users a graceful upgrade procedure when they have a fleet of configs they wish to migrate one at a time.

### Step 2 (V5)

- Delete the `try` and `catch` processors.
- Implement hard rejection mode in all processing pipeline execution functions.

