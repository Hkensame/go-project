package otgrpc

import (
	"io"
	"runtime"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	jaegerClient "github.com/uber/jaeger-client-go"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// OpenTracingClientInterceptor returns a grpc.UnaryClientInterceptor suitable
// for use in a grpc.Dial call.
//
// For example:
//
//	conn, err := grpc.Dial(
//	    address,
//	    ...,  // (existing DialOptions)
//	    grpc.WithUnaryInterceptor(otgrpc.OpenTracingClientInterceptor(tracer)))
//
// All gRPC client spans will inject the OpenTracing SpanContext into the gRPC
// metadata; they will also look in the context.Context for an active
// in-process parent Span and establish a ChildOf reference if such a parent
// Span could be found.
func OpenTracingClientInterceptor(tracer opentracing.Tracer, optFuncs ...Option) grpc.UnaryClientInterceptor {
	//把这个三方包给定的可选opt收集并在后面处理
	otgrpcOpts := newOptions()
	otgrpcOpts.apply(optFuncs...)
	return func(
		//这个ctx来自客户端dial产生的客户端连接调用rpc函数时传入的ctx
		ctx context.Context,
		//rpc函数方法名,用于复写到span中
		method string,
		req, resp interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		var err error

		//这里是作者留下的口子,允许外界传入parentSpan来实现跨微服务的链路追踪,
		//但局限是无法把tracer一并传入进来,
		var parentCtx opentracing.SpanContext
		if parent := opentracing.SpanFromContext(ctx); parent != nil {
			parentCtx = parent.Context()
		}

		//修改源码,把由web端装有tracer和parent-span的gin.Context打包的ctx拆出要的数据来
		gctxItface := ctx.Value("gin-ctx")
		switch gctxItface.(type) {
		case *gin.Context:
			gctx, ok := gctxItface.(*gin.Context)
			if ok {
				if tracerItface, exist := gctx.Get("tracer"); exist {
					tracer = tracerItface.(opentracing.Tracer)
				}
				if spanItface, exist := gctx.Get("web-span"); exist {
					parentCtx = spanItface.(*jaegerClient.Span).Context()
				}
			}
		}

		if otgrpcOpts.inclusionFunc != nil &&
			!otgrpcOpts.inclusionFunc(parentCtx, method, req, resp) {
			return invoker(ctx, method, req, resp, cc, opts...)
		}
		//生成子span,注意这里的子span是Finish过的,而Server那边的span也Finish过,
		//故在ui中将会看到两个一模一样的span段,如果觉得有影响,可以把server端的删去
		clientSpan := tracer.StartSpan(
			method,
			opentracing.ChildOf(parentCtx),
			ext.SpanKindRPCClient,
			gRPCComponentTag,
		)
		defer clientSpan.Finish()
		//把得到的子span和trace注入到发送给srv端的ctx,
		ctx = injectSpanContext(ctx, tracer, clientSpan)
		if otgrpcOpts.logPayloads {
			clientSpan.LogFields(log.Object("gRPC request", req))
		}
		err = invoker(ctx, method, req, resp, cc, opts...)
		if err == nil {
			if otgrpcOpts.logPayloads {
				clientSpan.LogFields(log.Object("gRPC response", resp))
			}
		} else {
			SetSpanTags(clientSpan, err, true)
			clientSpan.LogFields(log.String("event", "error"), log.String("message", err.Error()))
		}
		if otgrpcOpts.decorator != nil {
			otgrpcOpts.decorator(clientSpan, method, req, resp, err)
		}
		return err
	}
}

// OpenTracingStreamClientInterceptor returns a grpc.StreamClientInterceptor suitable
// for use in a grpc.Dial call. The interceptor instruments streaming RPCs by creating
// a single span to correspond to the lifetime of the RPC's stream.
//
// For example:
//
//	conn, err := grpc.Dial(
//	    address,
//	    ...,  // (existing DialOptions)
//	    grpc.WithStreamInterceptor(otgrpc.OpenTracingStreamClientInterceptor(tracer)))
//
// All gRPC client spans will inject the OpenTracing SpanContext into the gRPC
// metadata; they will also look in the context.Context for an active
// in-process parent Span and establish a ChildOf reference if such a parent
// Span could be found.
func OpenTracingStreamClientInterceptor(tracer opentracing.Tracer, optFuncs ...Option) grpc.StreamClientInterceptor {
	otgrpcOpts := newOptions()
	otgrpcOpts.apply(optFuncs...)
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		var err error
		var parentCtx opentracing.SpanContext
		if parent := opentracing.SpanFromContext(ctx); parent != nil {
			parentCtx = parent.Context()
		}
		if otgrpcOpts.inclusionFunc != nil &&
			!otgrpcOpts.inclusionFunc(parentCtx, method, nil, nil) {
			return streamer(ctx, desc, cc, method, opts...)
		}

		clientSpan := tracer.StartSpan(
			method,
			opentracing.ChildOf(parentCtx),
			ext.SpanKindRPCClient,
			gRPCComponentTag,
		)
		ctx = injectSpanContext(ctx, tracer, clientSpan)
		cs, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			clientSpan.LogFields(log.String("event", "error"), log.String("message", err.Error()))
			SetSpanTags(clientSpan, err, true)
			clientSpan.Finish()
			return cs, err
		}
		return newOpenTracingClientStream(cs, method, desc, clientSpan, otgrpcOpts), nil
	}
}

func newOpenTracingClientStream(cs grpc.ClientStream, method string, desc *grpc.StreamDesc, clientSpan opentracing.Span, otgrpcOpts *options) grpc.ClientStream {
	finishChan := make(chan struct{})

	isFinished := new(int32)
	*isFinished = 0
	finishFunc := func(err error) {
		// The current OpenTracing specification forbids finishing a span more than
		// once. Since we have multiple code paths that could concurrently call
		// `finishFunc`, we need to add some sort of synchronization to guard against
		// multiple finishing.
		if !atomic.CompareAndSwapInt32(isFinished, 0, 1) {
			return
		}
		close(finishChan)
		defer clientSpan.Finish()
		if err != nil {
			clientSpan.LogFields(log.String("event", "error"), log.String("message", err.Error()))
			SetSpanTags(clientSpan, err, true)
		}
		if otgrpcOpts.decorator != nil {
			otgrpcOpts.decorator(clientSpan, method, nil, nil, err)
		}
	}
	go func() {
		select {
		case <-finishChan:
			// The client span is being finished by another code path; hence, no
			// action is necessary.
		case <-cs.Context().Done():
			finishFunc(cs.Context().Err())
		}
	}()
	otcs := &openTracingClientStream{
		ClientStream: cs,
		desc:         desc,
		finishFunc:   finishFunc,
	}

	// The `ClientStream` interface allows one to omit calling `Recv` if it's
	// known that the result will be `io.EOF`. See
	// http://stackoverflow.com/q/42915337
	// In such cases, there's nothing that triggers the span to finish. We,
	// therefore, set a finalizer so that the span and the context goroutine will
	// at least be cleaned up when the garbage collector is run.
	runtime.SetFinalizer(otcs, func(otcs *openTracingClientStream) {
		otcs.finishFunc(nil)
	})
	return otcs
}

type openTracingClientStream struct {
	grpc.ClientStream
	desc       *grpc.StreamDesc
	finishFunc func(error)
}

func (cs *openTracingClientStream) Header() (metadata.MD, error) {
	md, err := cs.ClientStream.Header()
	if err != nil {
		cs.finishFunc(err)
	}
	return md, err
}

func (cs *openTracingClientStream) SendMsg(m interface{}) error {
	err := cs.ClientStream.SendMsg(m)
	if err != nil {
		cs.finishFunc(err)
	}
	return err
}

func (cs *openTracingClientStream) RecvMsg(m interface{}) error {
	err := cs.ClientStream.RecvMsg(m)
	if err == io.EOF {
		cs.finishFunc(nil)
		return err
	} else if err != nil {
		cs.finishFunc(err)
		return err
	}
	if !cs.desc.ServerStreams {
		cs.finishFunc(nil)
	}
	return err
}

func (cs *openTracingClientStream) CloseSend() error {
	err := cs.ClientStream.CloseSend()
	if err != nil {
		cs.finishFunc(err)
	}
	return err
}

func injectSpanContext(ctx context.Context, tracer opentracing.Tracer, clientSpan opentracing.Span) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	} else {
		md = md.Copy()
	}
	mdWriter := metadataReaderWriter{md}
	err := tracer.Inject(clientSpan.Context(), opentracing.HTTPHeaders, mdWriter)
	// We have no better place to record an error than the Span itself :-/
	if err != nil {
		clientSpan.LogFields(log.String("event", "Tracer.Inject() failed"), log.Error(err))
	}
	return metadata.NewOutgoingContext(ctx, md)
}
