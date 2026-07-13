<?php
// find_class_files.php
error_reporting(0);
ini_set('display_errors', 0);

$projectRoot = $argv[1] ?? '.';
if (file_exists($projectRoot . '/vendor/autoload.php')) {
    require_once $projectRoot . '/vendor/autoload.php';
}

// We also want to boot the Kernel to get all services autodiscovered
// but without actually running a command.
// We can't easily boot it here without knowing the Kernel class name.
// So we fallback to scanning the definitions from JSON but ADDING 
// a scan of the project directory via class_exists for potential candidates.

$json = file_get_contents('php://stdin');
$data = json_decode($json, true);

$mapping = [];
$classesToCheck = [];

// 1. Classes from definitions
foreach ($data['definitions'] ?? [] as $id => $def) {
    if (isset($def['class']) && $def['class']) {
        $classesToCheck[$def['class']] = true;
    }
}

// 2. Classes from aliases
foreach ($data['aliases'] ?? [] as $id => $target) {
    if (class_exists($id) || interface_exists($id) || trait_exists($id)) {
        $classesToCheck[$id] = true;
    }
}

// 3. Perform Reflection
foreach ($classesToCheck as $class => $_) {
    try {
        if (class_exists($class) || interface_exists($class) || trait_exists($class)) {
            $reflection = new ReflectionClass($class);
            $file = $reflection->getFileName();
            if ($file && file_exists($file)) {
                $mapping[$class] = $file;
            }
        }
    } catch (Throwable $e) {}
}

echo json_encode((object)$mapping);
