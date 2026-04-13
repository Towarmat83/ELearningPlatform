FROM alpine:3.19

RUN apk add --no-cache \
    bash \
    coreutils \
    grep \
    findutils \
    tar \
    gzip \
    python3 \
    shadow \
    util-linux \
 && adduser -D -s /bin/bash student

# ─── Generate environment with Python ────────────────────────────────────────
RUN python3 - <<'PYEOF'
import os, random, csv

# ── Fragment 1: grep an ALERT in a log file ─────────────────────────────────
os.makedirs("/var/log/nexus", exist_ok=True)
levels = ["INFO", "DEBUG", "WARN", "ERROR"]
lines = []
for i in range(1, 501):
    lvl = random.choice(levels)
    lines.append(f"2024-03-{(i%28)+1:02d} {(i%24):02d}:{(i%60):02d}:00 [{lvl}] service=api pid={1000+i} msg=\"Request processed in {i}ms\"\n")
# Plant exactly one ALERT with the first fragment
lines[247] = '2024-03-14 08:42:17 [ALERT] service=nexus-core pid=9912 msg="NEXUS_CODE_PART1=4a2f"\n'
with open("/var/log/nexus/system.log", "w") as f:
    f.writelines(lines)

# ── Fragment 2: grep a CSV and use cut to extract a field ───────────────────
os.makedirs("/opt/nexus/data", exist_ok=True)
rows = []
rows.append(["ID","NAME","STATUS","CODE","REGION"])
for i in range(1, 151):
    rows.append([str(i), f"AGENT_{i:04d}", random.choice(["ACTIVE","INACTIVE","PENDING"]), f"X{i:04d}", random.choice(["EU","US","AS"])])
# Plant the target row
rows[73] = ["73","NEXUS_AGENT","ACTIVE","b8c3","EU"]
with open("/opt/nexus/data/agents.csv", "w", newline="") as f:
    writer = csv.writer(f)
    writer.writerows(rows)

# ── Fragment 3: classified.txt — needs chmod to read ────────────────────────
with open("/opt/nexus/data/classified.txt", "w") as f:
    f.write("=== DOSSIER CLASSIFIE NEXUS ===\n")
    f.write("NIVEAU: TOP SECRET\n")
    f.write("FRAGMENT: 9d1e\n")
    f.write("=================================\n")

# ── Fragment 4: hidden archive ──────────────────────────────────────────────
os.makedirs("/opt/nexus/backup", exist_ok=True)
with open("/opt/nexus/backup/HUNT3R.txt", "w") as f:
    f.write("FRAGMENT_FINAL=HUNT3R\n")
    f.write("Félicitations, agent ! Vous avez trouvé le dernier fragment.\n")

print("Environment generated successfully")
PYEOF

# ── Create the hidden archive and clean up the source ─────────────────────────
RUN cd /opt/nexus && tar -czf data/.backup.tar.gz -C /opt/nexus/backup HUNT3R.txt \
 && rm -rf /opt/nexus/backup

# ── Set permissions ───────────────────────────────────────────────────────────
# classified.txt is readable only by root (student needs chmod)
RUN chown root:root /opt/nexus/data/classified.txt \
 && chmod 000 /opt/nexus/data/classified.txt \
# logs readable by student
 && chown -R student:student /var/log/nexus \
# data dir readable (but not classified.txt)
 && chown -R root:root /opt/nexus \
 && chmod 755 /opt/nexus/data \
# Ownership to student so chmod +r works without sudo
 && chown student:student /opt/nexus/data/classified.txt

# ── Welcome banner ────────────────────────────────────────────────────────────
RUN cat > /etc/motd <<'EOF'
╔══════════════════════════════════════════════════════╗
║          OPÉRATION NEXUS — LEARNLAB                  ║
╠══════════════════════════════════════════════════════╣
║  Bienvenue, agent. Votre mission :                   ║
║  Reconstituer le code d'accès NEXUS en 4 fragments.  ║
║                                                      ║
║  FORMAT DU FLAG : FLAG{xxxx_xxxx_xxxx_XXXXX}         ║
║                                                      ║
║  Indice 1 : Cherchez une alerte dans les logs        ║
║             → /var/log/nexus/system.log              ║
║  Indice 2 : Un agent spécial dans les données CSV    ║
║             → /opt/nexus/data/agents.csv             ║
║  Indice 3 : Un fichier classifié illisible...        ║
║             → /opt/nexus/data/classified.txt         ║
║  Indice 4 : Une archive cachée quelque part...       ║
║             → cherchez dans /opt/nexus avec find     ║
║             → extrayez avec : tar -xzf <archive> -C /tmp ║
╚══════════════════════════════════════════════════════╝
EOF

RUN echo 'cat /etc/motd' >> /home/student/.bashrc \
 && echo 'PS1="\[\e[1;32m\]student@nexus\[\e[0m\]:\[\e[1;34m\]\w\[\e[0m\]\$ "' >> /home/student/.bashrc

USER student
WORKDIR /home/student

CMD ["tail", "-f", "/dev/null"]
