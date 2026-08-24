<?php

namespace App\Service;

class DestructorLeakService
{
    private string $logPath;

    public function __construct()
    {
        // Use a persistent file on disk to trace life-cycle events across requests
        $this->logPath = __DIR__ . '/../../var/destructor_demo.log';
        
        $this->log("🟢 Constructor called");
    }

    public function __destruct()
    {
        $this->log("🔴 Destructor called");
    }

    public function getLogContent(): string
    {
        if (!file_exists($this->logPath)) {
            return "No events recorded yet.";
        }
        return file_get_contents($this->logPath);
    }

    public function clearLog(): void
    {
        if (file_exists($this->logPath)) {
            unlink($this->logPath);
        }
    }

    private function log(string $message): void
    {
        $time = date('H:i:s');
        $pid = getmypid();
        $mode = php_sapi_name() === 'cli' ? 'WORKER' : 'CLASSIC/FPM';
        // Append life-cycle event to file
        file_put_contents(
            $this->logPath,
            "[$time] [PID: $pid] $message\n",
            FILE_APPEND | LOCK_EX
        );
    }
}
