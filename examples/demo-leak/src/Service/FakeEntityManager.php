<?php

namespace App\Service;

class FakeFilters
{
    private array $disabledFilters = [];

    public function disable(string $name): void
    {
        $this->disabledFilters[$name] = true;
    }

    public function enable(string $name): void
    {
        unset($this->disabledFilters[$name]);
    }

    public function isEnabled(string $name): bool
    {
        return !isset($this->disabledFilters[$name]);
    }
}

class FakeEntityManager
{
    private FakeFilters $filters;

    public function __construct()
    {
        $this->filters = new FakeFilters();
    }

    public function getFilters(): FakeFilters
    {
        return $this->filters;
    }
}
