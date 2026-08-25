<?php

namespace App\Controller;

use Vendor\Lib\CalculatorInterface;

class InterfaceController
{
    public function __construct(private CalculatorInterface $calculator)
    {
    }

    public function run(): void
    {
        $this->calculator->add(1);
    }
}
