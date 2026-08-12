<?php

namespace App\Service;

class FakeSpan implements Span {
    public function setAttribute(string $key, string $value): void {}
}
