<?php

namespace Vendor\Lib;

class InheritedBaseService
{
    private array $data = [];

    public function mutate(string $value): void
    {
        $this->data[] = $value;
    }

    public function neverCalled(): void
    {
        $this->data[] = 'unused';
    }
}

class InheritedChildService extends InheritedBaseService
{
    // Inherits mutate() and neverCalled() without overriding either.
}

class InheritedGrandchildService extends InheritedChildService
{
    // Multi-level inheritance: still does not override anything.
}

class OverriddenBaseService
{
    private array $data = [];

    public function mutate(string $value): void
    {
        $this->data[] = $value;
    }
}

class OverridingChildService extends OverriddenBaseService
{
    private array $ownData = [];

    public function mutate(string $value): void
    {
        $this->ownData[] = $value;
    }
}
