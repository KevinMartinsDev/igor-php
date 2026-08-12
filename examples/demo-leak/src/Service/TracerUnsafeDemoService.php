<?php

namespace App\Service;

class TracerUnsafeDemoService {
    private TracerInterface $tracer;

    public function __construct(TracerInterface $tracer) {
        $this->tracer = $tracer;
    }

    public function handleUnsafeTracing(): void {
        // Unsafe: getSharedManager() returns FakeEntityManager, which is a shared service!
        $em = $this->tracer->getSharedManager();
        $em->getFilters()->disable('softdeleteable'); // Should trigger mutation finding on local shared reference
    }
}
