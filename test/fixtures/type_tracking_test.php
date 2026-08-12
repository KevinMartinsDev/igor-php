<?php

namespace App\Service\TraceTest;

interface TracerInterface {
    public function makeSpan(): Span;
    public function getSharedManager(): FakeEntityManager;
}

interface Span {
    public function setAttribute(string $key, string $value): void;
}

class FakeEntityManager {
    public function getFilters() {
        return new FakeFilters();
    }
}

class FakeFilters {
    public function disable(string $name): void {}
}

class SuperService {
    private \Doctrine\ORM\EntityManagerInterface $em;
    private TracerInterface $tracer;

    public function __construct(\Doctrine\ORM\EntityManagerInterface $em, TracerInterface $tracer) {
        $this->em = $em;
        $this->tracer = $tracer;
    }

    public function testRemove($product) {
        // Safe direct call on whitelisted EM method (Chantier 1)
        $this->em->remove($product);
    }

    public function testTransientSpan() {
        // Safe: Span is returned by Tracer and is not a shared service (Chantier 2)
        $span = $this->tracer->makeSpan();
        $span->setAttribute('key', 'value');
    }

    public function testChainedEM() {
        // Unsafe chained call (still flagged)
        $this->em->getFilters()->disable('softdeleteable');
    }
}

class UnsafeService {
    private TracerInterface $tracer;

    public function __construct(TracerInterface $tracer) {
        $this->tracer = $tracer;
    }

    public function testUnsafeTracing() {
        // Unsafe: getSharedManager() returns FakeEntityManager, which is a shared service!
        $em = $this->tracer->getSharedManager();
        $em->getFilters()->disable('softdeleteable'); // Should trigger mutation finding on local shared reference
    }
}
