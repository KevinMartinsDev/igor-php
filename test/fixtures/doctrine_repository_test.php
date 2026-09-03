<?php

namespace App\Repository;

class ServiceEntityRepository {
    private \Doctrine\ORM\EntityManagerInterface $em;

    public function __construct(\Doctrine\ORM\EntityManagerInterface $em) {
        $this->em = $em;
    }

    public function getEntityManager(): \Doctrine\ORM\EntityManagerInterface {
        return $this->em;
    }
}

class ProductRepository extends ServiceEntityRepository {
    public function testPrune($product) {
        // Safe: $manager type is resolved via getEntityManager() in the parent class
        $manager = $this->getEntityManager();
        $manager->remove($product);
    }

    public function testUnsafeMutation() {
        // Unsafe: setUnsafeProperty starts with mutation prefix 'set' and is not whitelisted for Doctrine EntityManager
        $manager = $this->getEntityManager();
        $manager->setUnsafeProperty('value');
    }
}
