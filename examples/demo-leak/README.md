# 🧟‍♂️ Igor-Php: Global State Leak Demo

This project demonstrates several ways PHP state can "leak" between requests when running in a persistent environment like FrankenPHP Worker mode.

## 🧪 The Experiments

Start the project using Docker:
```bash
docker compose up -d
```
Then visit [http://localhost:8080](http://localhost:8080) to access the **Igor Leak Lab**.

### 🔍 What to look for?
In this laboratory, **NOTHING** is stored in a database, session, cookie, or cache file. 
Everything you see is stored **exclusively in PHP's RAM**.

1.  **Stateful Service Leak**: Property mutation without ResetInterface.
2.  **Incomplete Reset Leak**: Implementing ResetInterface but forgetting a property.
3.  **Static Property Leak**: How static data survives everything (Class level).
4.  **Global State Poisoning**: Changing process settings (Timezone).
5.  **Memory Pressure**: See how RAM usage grows and STAYS high.
6.  **The Danger of Exit/Die**: Killing the worker thread.
7.  **Shared Service Indirect Mutation**: Disabling filters on an EntityManager instance fetched into a local variable, showing how its state is modified permanently for all future requests.

## 🛡️ How Igor-Php helps

Igor is designed to catch all these issues automatically. Run it on this project:

```bash
igor-php examples/demo-leak
```

## 🧹 Cleanup
```bash
docker compose down
```
