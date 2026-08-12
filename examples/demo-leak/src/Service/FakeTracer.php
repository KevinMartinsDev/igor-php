<?php

namespace App\Service;

class   FakeTracer implements TracerInterface {
    public function makeSpan(): Span {
        return new FakeSpan();
    }

    public function getSharedManager(): FakeEntityManager {
        return new FakeEntityManager();
    }
}
