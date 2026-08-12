<?php

namespace App\Service;

interface TracerInterface {
    public function makeSpan(): Span;
    public function getSharedManager(): FakeEntityManager;
}
