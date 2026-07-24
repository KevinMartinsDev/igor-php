<?php

namespace IgorPhp\IgorBundle\DependencyInjection\Compiler;

use Symfony\Component\DependencyInjection\Compiler\CompilerPassInterface;
use Symfony\Component\DependencyInjection\ContainerBuilder;
use Symfony\Component\DependencyInjection\Definition;
use Symfony\Component\DependencyInjection\Reference;

class IgorDiscoveryPass implements CompilerPassInterface
{
    public function process(ContainerBuilder $container): void
    {
        $serviceMap = [
            'definitions' => [],
            'aliases' => [],
        ];

        foreach ($container->getDefinitions() as $id => $definition) {
            if ($definition->isSynthetic() || !$definition->getClass()) {
                continue;
            }

            $class = $container->getParameterBag()->resolveValue($definition->getClass());
            $isResettable = $definition->hasTag('kernel.reset');
            if (!$isResettable && $class) {
                try {
                    if (class_exists($class)) {
                        $isResettable = is_subclass_of($class, 'Symfony\Contracts\Service\ResetInterface');
                    }
                } catch (\Throwable $e) {
                    $isResettable = false;
                }
            }

            $serviceMap['definitions'][$id] = [
                'class' => $class,
                'public' => $definition->isPublic(),
                'shared' => $definition->isShared(),
                'resettable' => $isResettable,
                'arguments' => $this->extractDependencies($definition),
            ];
        }

        foreach ($container->getAliases() as $id => $alias) {
            $serviceMap['aliases'][$id] = (string) $alias;
        }

        $cacheDir = $container->getParameter('kernel.cache_dir');
        if (!is_dir($cacheDir)) {
            mkdir($cacheDir, 0777, true);
        }

        file_put_contents(
            $cacheDir . '/igor_service_map.json',
            json_encode($serviceMap, JSON_PRETTY_PRINT)
        );
    }

    private function extractDependencies(Definition $definition): array
    {
        $deps = [];

        // Constructor arguments
        foreach ($definition->getArguments() as $arg) {
            if ($arg instanceof Reference) {
                $deps[] = ['type' => 'service', 'id' => (string) $arg];
            }
        }

        // Setter injection / Method calls
        foreach ($definition->getMethodCalls() as $call) {
            foreach ($call[1] as $arg) {
                if ($arg instanceof Reference) {
                    $deps[] = ['type' => 'service', 'id' => (string) $arg];
                }
            }
        }

        return $deps;
    }
}
