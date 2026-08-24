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

class GeneratorBaseService
{
    private array $temporaryFiles = [];
    private array $renderCache = [];

    public function getOutputFromHtml(string $html): string
    {
        $this->renderCache[] = $html;
        return $this->createTemporaryFile();
    }

    public function createTemporaryFile(): string
    {
        $this->temporaryFiles[] = 'tmp';
        return 'tmp';
    }
}

class GeneratorMiddleService extends GeneratorBaseService
{
    // Inherits getOutputFromHtml() and createTemporaryFile() without overriding either.
}

class GeneratorLeafService extends GeneratorMiddleService
{
    // Third inheritance level: still does not override anything. Only
    // getOutputFromHtml() is called directly from app code; promoting it to
    // GeneratorBaseService must also make the graph edge to
    // createTemporaryFile() (reached only via `$this->` on the ancestor)
    // reachable in turn.
}
