<?php

namespace App\Service;

interface Span {
    public function setAttribute(string $key, string $value): void;
}
