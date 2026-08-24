<?php

class DestructorClass {
    public function __destruct() {
        // This is dangerous and should be flagged
    }
}

class IgnoredDestructorClass {
    /** @igor-ignore */
    public function __destruct() {
        // This is dangerous but has @igor-ignore, so it should not be flagged
    }
}

#[WorkerSafe]
class SafeClass {
    public function __destruct() {
        // Safe: class is WorkerSafe
    }
}

class SafeMethodClass {
    #[WorkerSafe]
    public function __destruct() {
        // Safe: method is WorkerSafe
    }
}
