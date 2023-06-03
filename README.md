# Hyperkaehler
This is a bot that tries to make mana on [Manifold Markets.](https://manifold.markets)

Right now it's just trying to farm streak bonuses. Eventually I'll implement some kind of actual strategy.

## Instructions
`docker build -t hyperkaehler .`

`docker run -p 8080:80 -p 8443:443 hyperkaehler`