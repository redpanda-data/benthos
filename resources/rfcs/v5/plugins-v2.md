Plugins V2
==========

## Replacing the Connect Method

### Problem

Right now we have a non-trivial flow that plugins must implement:

1. Plugin constructor called, config validation and linting should be done
2. Connect is called, the definition of a "connection" is vague and will differ between plugins. If an error is returned Connect will be called again with backoff.
3. Read/Write is called depending on whether the plugin is an input or output. If an ErrNotConnected is returned then we go back to step 2.
4. Close is called, whereby the plugin should close itself and no other methods will be called.

The relationship here between stages 2 and 3 are confusing and have caused very simple plugin implementations to become convoluted in an attempt to "adhere" to it.

### Why the Problem Exists

Traditionally the Connect method serves two purposes, it indicates to the plugin that we are ready for it to begin connecting to its external services. It also indicates (depending on the return value) whether or not the plugin is currently successfully connected. However, this is inaccurate and often cannot be applied to plugin implementations where an underlying library "owns" the connectivity status.

### Proposed Solution

We should deprecate `Connect` and add two new methods that replace its behavior. The first will be `Init`, which is a method responsible for indicating to a plugin that it is safe to establish external connections. If `Init` returns an error then this will be interpretted as a signal that the plugin configuration is invalid and the pipeline should be aborted.

The second method will be `ConnectionStatus`, which provides the engine a way of determining whether the plugin is actively connected. It will be the responsbility of plugin implementations to find the most appropriate way of tracking this information.

## Processors Get Connectivity Treatment

Processors have traditionally been ignored when it comes to connectivity detection. Since we're expanding our plugin APIs we should also add these new methods to processors, and caches and ratelimits as well whilst we're at it.

## TestConnection Method

Once we have an explicit `Init` stage for all plugins it'll mean plugins can be expected to remain dormant once constructed. This gives us the ability to add functionality to plugins that is complementary without risking unwanted connections.

One such example of this is the ability to test the connectivity of a plugin with a given configuration without actually kicking off a consumer/producer. This new method will be called `TestConnection` and will be added to all plugin types.

We can choose to make this optional in order to reduce the burden on plugin authors.

## Switching Outputs to a Pull System

Note: Theres some cool iter APIs in Go now to check out: https://pkg.go.dev/iter

Currently outputs implement a `Write` or `WriteBatch` method, which takes a message or a batch of messages respectively, and the output is expected to deliver the data before returning an error. This system makes it awkward for outputs to implement their own asynchronous dispatch or batching mechanisms.

Instead we should consider adding a new method that provides a form of iterator API for messages, which would be the interface outputs extract messages from at their own discretion. This would make it much simpler for example to create a size based batching writer.

### Acknowledgements and Retries

Since providing an iterator would be yielding control from the engine as to when a write occurs we will need to have utilities internally for wrapping iterators with our own mechanisms for aggregating acknowledgements and write errors. This is likely a significant effort and would therefore need to be implemented in stages over a long period of time.

### Example

Some pseudocode that illustrates the idea around a batching mechanism:

```go
func (m *Meow) Init(ctx context.Context, iter Iterator) error {
    conn, err := m.getConnection(ctx)
    if err != nil {
        return err
    }

    go func() {
        defer m.shutSig.ShutdownComplete()

    batchLoop:
        for {
            var batch service.MessageBatch
            var ackFns service.AckFuncs
            var batchLen int

            for batchLen < batchSize {
                msg, ackFn, ok := iter.Next()
                if !ok {
                    return
                }

                batch = append(batch, msg)
                ackFns = append(ackFns, ackFn)

                batchLen += len(msg.AsBytes())
            }

            m.Flush(batch, ackFns)
            continue batchLoop
        }
    }()

    return nil
}
```

## Implementation

### Step 1 (V4)

- We add a new interface for each component type containing the new suite of methods but otherwise matching the flow of the existing plugins.
  + The new output APIs will be suffixed `Push` to indicate they are push based but will otherwise match the existing output in order to ease the transition.
  + The `TestConnection` method is optional at this point for all plugin types.
- We migrate as many existing plugins to the new APIs as we can, noting any awkwardnesses and feedback as we go.
- Once our confidence grows we mark the old APIs as deprecated.
- We add a new `Pull` suffixed output interface that uses iterators, and migrate a few of our output plugins to it.

### Step 2 (V5)

- If we managed to deprecate the old APIs with plenty of time before V5 then we can delete them. I would say at least a few months would be appropriate as rewriting plugins is a big ask.
- We can choose to make the `TestConnection` method mandatory at this point based on customer feedback.

