<?php

namespace Vendor\Lib;

interface CalculatorInterface
{
    public function add(int $value): void;
}

class Calculator implements CalculatorInterface
{
    private int $total = 0;

    public function add(int $value): void
    {
        $this->total += $value;
    }

    public function neverCalled(): void
    {
        $this->total = 0;
    }
}
