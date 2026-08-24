<?php

namespace App\Controller;

use Vendor\Lib\InheritedChildService;
use Vendor\Lib\InheritedGrandchildService;
use Vendor\Lib\OverridingChildService;
use Vendor\Lib\GeneratorLeafService;

class InheritanceController
{
    private InheritedChildService $inheritedChildService;
    private InheritedGrandchildService $inheritedGrandchildService;
    private OverridingChildService $overridingChildService;
    private GeneratorLeafService $generatorLeafService;

    public function run(): void
    {
        $this->inheritedChildService->mutate('a');
        $this->inheritedGrandchildService->mutate('b');
        $this->overridingChildService->mutate('c');
        $this->generatorLeafService->getOutputFromHtml('<p>x</p>');
    }
}
