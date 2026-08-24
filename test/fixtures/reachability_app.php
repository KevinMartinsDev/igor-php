<?php

namespace App\Controller;

use Vendor\Lib\VendorService;

class AppController
{
    private VendorService $vendorService;

    public function run(): void
    {
        $this->vendorService->entryPoint('hello');
    }
}
