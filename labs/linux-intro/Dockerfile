FROM alpine:3.19

# Install tools available to the student
RUN apk add --no-cache bash coreutils grep findutils file

# ── Challenge setup ────────────────────────────────────────────────────────────

# Stage 1: visible hint in home directory
RUN mkdir -p /home/student && \
    cat > /home/student/README.md <<'EOF'
# Opération SHADOW

Bienvenue, agent.

Une taupe a laissé des traces sur ce système. Votre mission :
retrouver le code secret qu'elle a caché dans les profondeurs du filesystem.

Points de départ :
- Explorez /opt/mission/
- Les fichiers cachés (`.`) peuvent contenir des surprises
- Certaines données sont encodées — pensez à `base64 -d`

Bonne chance.
EOF

# Stage 1: public file with first clue
RUN mkdir -p /opt/mission/data && \
    echo "Niveau 1 : bien joué. Cherchez maintenant dans les répertoires cachés de /opt/mission/" \
    > /opt/mission/data/niveau1.txt && \
    echo "Indices de mission récupérés." > /opt/mission/data/rapport.txt

# Stage 2: hidden directory with second clue
RUN mkdir -p /opt/mission/.archive && \
    echo "Niveau 2 : presque... Le flag est encodé quelque part dans /var/cache/" \
    > /opt/mission/.archive/niveau2.txt

# Stage 3: flag encoded in base64, hidden in a deeper path
RUN mkdir -p /var/cache/shadow && \
    echo "FLAG{linux_sh4d0w_hunt3r_2024}" | base64 > /var/cache/shadow/.secret.b64 && \
    chmod 644 /var/cache/shadow/.secret.b64

# Decoy files to make exploration more interesting
RUN echo "nothing here" > /opt/mission/data/todo.txt && \
    echo "rapport confidentiel — accès refusé" > /opt/mission/data/classified.txt && \
    mkdir -p /var/cache/apt && \
    echo "cache apt standard" > /var/cache/apt/pkgcache.bin && \
    echo "just a log" > /var/log/syslog

# Create a non-privileged user for the session
RUN adduser -D -s /bin/bash student && \
    chown -R student:student /home/student

USER student
WORKDIR /home/student

# Keep the container alive — the shell is launched via docker exec by the API
CMD ["tail", "-f", "/dev/null"]
