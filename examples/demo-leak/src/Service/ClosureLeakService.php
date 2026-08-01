<?php

namespace App\Service;

class ClosureLeakService
{
    private array $listeners = [];

    public function addListener(string $event, callable $listener): void
    {
        $this->listeners[$event][] = $listener;
    }

    public function getListeners(): array
    {
        return $this->listeners;
    }
}
