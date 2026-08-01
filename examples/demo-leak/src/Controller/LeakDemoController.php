<?php

namespace App\Controller;

use App\Service\FakeEntityManager;
use App\Service\IncompleteResetService;
use App\Service\StatefulService;
use App\Service\StaticLeakService;
use Symfony\Bundle\FrameworkBundle\Controller\AbstractController;
use Symfony\Component\HttpFoundation\Response;
use Symfony\Component\Routing\Attribute\Route;

class LeakDemoController extends AbstractController
{
    public function __construct(
        private StatefulService $statefulService,
        private IncompleteResetService $incompleteResetService,
        private StaticLeakService $staticLeakService,
        private FakeEntityManager $entityManager,
    ) {}

    #[Route('/', name: 'demo_index')]
    public function index(): Response
    {
        return $this->renderLayout("
            <h1>🧟 Igor Leak Lab</h1>
            <div style='background: #fff3cd; padding: 15px; border: 1px solid #ffeeba; border-radius: 5px; margin-bottom: 20px;'>
                <b>⚠️ Important Note:</b> Everything you see is stored <b>exclusively in PHP's RAM</b>. 
            </div>
            <p>Welcome, Master. Pick an experiment:</p>
            <ul>
                <li><a href='/stateful-service'>1. Stateful Service Leak</a></li>
                <li><a href='/incomplete-reset'>2. Incomplete Reset Leak</a></li>
                <li><a href='/static-leak'>3. Static Property Leak</a></li>
                <li><a href='/check-timezone'>4. Global State Poisoning</a></li>
                <li><a href='/heavy-load' style='color: #dc3545; font-weight: bold;'>5. 🎮 CHALLENGE: Out of Memory Game</a></li>
                <li><a href='/exit'>6. The Danger of Exit/Die</a></li>
                <li><a href='/doctrine-leak' style='color: #28a745; font-weight: bold;'>7. Shared Service Indirect Mutation (Doctrine Filters Leak)</a></li>
            </ul>
        ");
    }

    #[Route('/stateful-service')]
    public function stateful(): Response
    {
        $this->statefulService->addData('req_' . time() . '_' . rand(1,1000), 'I was here!');
        $html = "<h2>1. Stateful Service</h2>
                 <button onclick='window.location.reload()' style='padding: 10px 20px; background: #007bff; color: white; border: none; border-radius: 5px; cursor: pointer; font-weight: bold;'>➕ Add Data (F5)</button>
                 <pre style='background: #f8f9fa; padding: 10px; border: 1px solid #ddd; margin-top: 10px;'>".print_r($this->statefulService->getData(), true)."</pre>";
        return $this->renderLayout($html, true);
    }

    #[Route('/incomplete-reset')]
    public function incomplete(): Response
    {
        $this->incompleteResetService->addData('Value ' . time());
        $html = "<h2>2. Incomplete Reset</h2>
                 <button onclick='window.location.reload()' style='padding: 10px 20px; background: #007bff; color: white; border: none; border-radius: 5px; cursor: pointer; font-weight: bold;'>⚡ Trigger Request (F5)</button>
                 <pre style='background: #f8f9fa; padding: 10px; border: 1px solid #ddd; margin-top: 10px;'>".print_r($this->incompleteResetService->getState(), true)."</pre>";
        return $this->renderLayout($html, true);
    }

    #[Route('/static-leak')]
    public function staticLeak(): Response
    {
        $this->staticLeakService->touch('User_' . rand(1, 100));
        $html = "<h2>3. Static Property Leak</h2>
                 <button onclick='window.location.reload()' style='padding: 10px 20px; background: #007bff; color: white; border: none; border-radius: 5px; cursor: pointer; font-weight: bold;'>🔄 Add Random User (F5)</button>
                 <pre style='background: #f8f9fa; padding: 10px; border: 1px solid #ddd; margin-top: 10px;'>".print_r($this->staticLeakService->get(), true)."</pre>";
        return $this->renderLayout($html, true);
    }

    #[Route('/heavy-load')]
    public function heavyLoad(): Response
    {
        // Allocate 5MB of random data to make it faster to crash
        $this->statefulService->addData('heavy_' . uniqid(), str_repeat('A', 5 * 1024 * 1024));
        
        $limit = ini_get('memory_limit');
        $html = "<h2>5. 🎮 CHALLENGE: Out of Memory</h2>
                 <div style='background: #eee; padding: 20px; border-radius: 10px;'>
                    <p style='font-size: 1.2em; text-align: center;'>PHP Memory Limit: <b>$limit</b></p>
                    
                    <div style='background: #fff; padding: 15px; border-left: 5px solid #dc3545; margin-bottom: 20px;'>
                        <b>💡 Real-world scenario:</b> Imagine this button is a <code>private array \$cache = []</code> in your service. 
                        Every time you fetch an entity from the database, you store it in this array to 'go faster'. 
                        But since you <b>never empty it</b> and the service <b>never dies</b>, your RAM will eventually explode.
                    </div>

                    <div style='text-align: center;'>
                        <p>Each click injects <b>5MB</b> into the <code>StatefulService</code> cache array.</p>
                        <button onclick='window.location.reload()' style='padding: 20px 40px; background: #dc3545; color: white; border: none; border-radius: 10px; cursor: pointer; font-size: 1.5em; font-weight: bold; box-shadow: 0 4px #900;'>💥 BOOM! (Add 5MB)</button>
                        <p style='margin-top: 20px; color: #666;'><i>Click until you see a 500 error. That's a memory leak in action!</i></p>
                    </div>
                 </div>";
        return $this->renderLayout($html, true);
    }

    #[Route('/check-timezone')]
    public function checkTimezone(): Response
    {
        $tz = date_default_timezone_get();
        $html = "<h2>4. Global Timezone</h2>
                 <p>Current process timezone: <b>$tz</b></p>
                 <a href='/poison-timezone' style='display: inline-block; padding: 10px 20px; background: #ffc107; color: black; text-decoration: none; border-radius: 5px; font-weight: bold;'>☣️ Inject America/New_York</a>
                 <a href='/check-timezone' style='display: inline-block; padding: 10px 20px; background: #6c757d; color: white; text-decoration: none; border-radius: 5px; font-weight: bold; margin-left: 10px;'>🔍 Refresh Status</a>";
        return $this->renderLayout($html, true);
    }

    #[Route('/poison-timezone')]
    public function poisonTimezone(): Response
    {
        date_default_timezone_set('America/New_York');
        return new Response("
            <body style='font-family: sans-serif; padding: 20px; text-align: center; padding-top: 50px;'>
                <h1>⚡ Poison injected!</h1>
                <p>Timezone changed for this worker thread.</p>
                <a href='/check-timezone' style='padding: 10px 20px; background: #28a745; color: white; text-decoration: none; border-radius: 5px; font-weight: bold;'>⬅️ Go back and check</a>
            </body>
        ");
    }

    #[Route('/exit')]
    public function exitDemo(): Response
    {
        echo "
            <body style='font-family: sans-serif; padding: 20px; text-align: center; padding-top: 50px;'>
                <h1 style='color: #dc3545;'>💀 Worker Terminated</h1>
                <p>Process PID " . getmypid() . " was killed.</p>
                <a href='/' style='padding: 10px 20px; background: #007bff; color: white; text-decoration: none; border-radius: 5px; font-weight: bold;'>⬅️ Restart & Back to Lab</a>
            </body>";
        exit();
    }

    #[Route('/doctrine-leak')]
    public function doctrineLeak(): Response
    {
        $isEnabled = $this->entityManager->getFilters()->isEnabled('softdeleteable');
        $statusStr = $isEnabled ? "<span style='color: #28a745; font-weight: bold;'>ENABLED (Safe)</span>" : "<span style='color: #dc3545; font-weight: bold;'>DISABLED (LEAKING!)</span>";
        
        $html = "<h2>7. Shared Service Indirect Mutation (Doctrine Filters)</h2>
                 <p>Doctrine softdeleteable filter is currently: $statusStr</p>
                 <div style='background: #f8f9fa; padding: 15px; border-left: 5px solid #007bff; margin-bottom: 20px;'>
                    <b>🧠 The Leak scenario:</b> Imagine a filter that resolves the <code>EntityManager</code> from the registry (into a local variable) 
                    and then disables the softdeleteable filter. Because the <code>EntityManager</code> is a shared service (Singleton), 
                    the filter is now disabled in the <b>entire process memory</b> for all subsequent requests!
                 </div>
                 <a href='/poison-filters' style='display: inline-block; padding: 10px 20px; background: #dc3545; color: white; text-decoration: none; border-radius: 5px; font-weight: bold;'>☣️ Disable Filter (via Local Variable)</a>
                 <a href='/doctrine-leak' style='display: inline-block; padding: 10px 20px; background: #6c757d; color: white; text-decoration: none; border-radius: 5px; font-weight: bold; margin-left: 10px;'>🔍 Refresh Status</a>";
        return $this->renderLayout($html, true);
    }

    #[Route('/poison-filters')]
    public function poisonFilters(): Response
    {
        // We simulate resolving the entityManager into a local variable and mutating its shared state!
        $entityManager = $this->entityManager;
        $entityManager->getFilters()->disable('softdeleteable');
        
        return new Response("
            <body style='font-family: sans-serif; padding: 20px; text-align: center; padding-top: 50px;'>
                <h1 style='color: #dc3545;'>⚡ Poison injected!</h1>
                <p>The softdeleteable filter has been disabled on the shared EntityManager singleton in RAM.</p>
                <a href='/doctrine-leak' style='padding: 10px 20px; background: #28a745; color: white; text-decoration: none; border-radius: 5px; font-weight: bold;'>⬅️ Go back and check the leak!</a>
            </body>
        ");
    }

    private function renderLayout(string $content, bool $showBack = false): Response
    {
        $mem = number_format(memory_get_usage() / 1024 / 1024, 3);
        $peak = number_format(memory_get_peak_usage() / 1024 / 1024, 3);
        $back = $showBack ? "<br><br><a href='/' style='color: #666;'>⬅️ Back to Lab</a>" : "";
        
        $html = "
            <html>
            <body style='font-family: sans-serif; padding: 20px; line-height: 1.6;'>
                $content 
                $back 
                <hr style='margin-top: 50px;'>
                <div style='background: #333; color: #0f0; padding: 15px; font-family: monospace; border-radius: 5px; box-shadow: 0 4px 8px rgba(0,0,0,0.2);'>
                    <b>[ IGOR MEMORY MONITOR ]</b><br>
                    <span style='color: #fff;'>Current RAM:</span> {$mem} MB<br>
                    <span style='color: #fff;'>Peak RAM:</span>    {$peak} MB<br>
                    <span style='color: #fff;'>Worker PID:</span>   " . getmypid() . "
                </div>
            </body>
            </html>";
        return new Response($html);
    }
}
