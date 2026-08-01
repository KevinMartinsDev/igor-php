<?php

namespace App\Service;

class LocalStaticService
{
    public function incrementAndGet(): int
    {
        static $counter = 0;
        $counter++;
        return $counter;
    }
}
