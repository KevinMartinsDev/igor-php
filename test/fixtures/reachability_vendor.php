<?php

namespace Vendor\Lib;

class VendorService
{
    private array $data = [];
    private array $other = [];

    public function entryPoint(string $value): void
    {
        $this->mutate($value);
    }

    public function mutate(string $value): void
    {
        $this->data[] = $value;
    }

    public function neverCalled(): void
    {
        $this->other[] = 'x';
    }
}
