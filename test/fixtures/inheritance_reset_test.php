<?php

namespace App\Service;

use Symfony\Contracts\Service\ResetInterface;

abstract class AbstractParentAdapter implements ResetInterface
{
    protected $sale;

    public function reset(): void
    {
        unset($this->sale);
    }
}

class ConcreteChildAdapter extends AbstractParentAdapter implements ResetInterface
{
    public function setSale($s)
    {
        $this->sale = $s; // Mutation of inherited property 'sale' - should NOT flag
    }
}

class ConcreteChildAdapterWithLocalLeak extends AbstractParentAdapter implements ResetInterface
{
    private $localProp; // Locally declared property

    public function setSale($s)
    {
        $this->sale = $s; // Mutation of inherited property 'sale' - should NOT flag
    }

    public function setLocalProp($val)
    {
        $this->localProp = $val; // Mutation of local property - should flag because not reset!
    }
}
