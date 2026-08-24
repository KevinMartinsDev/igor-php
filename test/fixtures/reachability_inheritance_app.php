<?php

namespace App\Controller;

use Vendor\Lib\InheritedChildService;
use Vendor\Lib\InheritedGrandchildService;
use Vendor\Lib\OverridingChildService;

class InheritanceController
{
    private InheritedChildService $inheritedChildService;
    private InheritedGrandchildService $inheritedGrandchildService;
    private OverridingChildService $overridingChildService;

    public function run(): void
    {
        $this->inheritedChildService->mutate('a');
        $this->inheritedGrandchildService->mutate('b');
        $this->overridingChildService->mutate('c');
    }
}
