<?php

namespace App\Service;

class TracerLeakDemoService {
    private TracerInterface $tracer;

    public function __construct(TracerInterface $tracer) {
        $this->tracer = $tracer;
    }

    public function handleTracing(): void {
        $span = $this->tracer->makeSpan();
        $span->setAttribute('key', 'value'); // Safe: Span is transient
    }
}
