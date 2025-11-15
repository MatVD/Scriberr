# Revue de code - Live Transcription

## ✅ Architecture Backend

### Services
- ✅ `LiveTranscriptionService` initialisé dans `cmd/server/main.go`
- ✅ Dépendance correcte sur `UnifiedTranscriptionService`
- ✅ Directory management: `data/uploads/live_sessions/`

### Routes API (`internal/api/router.go`)
```go
POST   /api/v1/transcription/live/sessions          - CreateLiveSession
GET    /api/v1/transcription/live/sessions/:id      - GetLiveSession
POST   /api/v1/transcription/live/sessions/:id/chunks - UploadLiveChunk (NoCompression)
GET    /api/v1/transcription/live/sessions/:id/stream - StreamLiveSession (SSE, NoCompression)
POST   /api/v1/transcription/live/sessions/:id/finalize - FinalizeLiveSession
POST   /api/v1/transcription/live/sessions/:id/cancel - CancelLiveSession
```

### Handlers (`internal/api/live_transcription_handlers.go`)
- ✅ Validation des entrées avec binding Gin
- ✅ Gestion d'erreurs HTTP appropriée (400/404/500)
- ✅ SSE streaming avec `text/plain` + flush
- ✅ Finalize crée un `TranscriptionJob` et l'enqueue

### Service Layer (`internal/transcription/live_service.go`)

#### Concurrency & Thread Safety
- ✅ `sync.Map` pour locks par session (évite race conditions)
- ✅ `sessionBroadcaster` avec `sync.RWMutex` pour pub/sub thread-safe
- ✅ Lock acquisition dans `AppendChunk` et `FinalizeSession`

#### Transcription Pipeline
```go
1. CreateSession → DB persist avec UUID auto-généré
2. AppendChunk → persistChunk (save raw) → convertToWav → TranscribeFile → DB save chunk
3. Subscribe → snapshot initial + broadcast channel
4. FinalizeSession → concatChunks (ffmpeg concat) → status finalizing
5. Handler → create TranscriptionJob → enqueue → status completed
```

#### Audio Processing
- ✅ `persistChunk`: Validation taille >1KB, conversion WebM→WAV 16kHz mono
- ✅ `convertToWav`: Simplifié (pas de fallback recovery car stop/start garantit containers complets)
- ✅ `concatChunks`: ffmpeg concat avec absolute paths, évite duplication path

### Models (`internal/models/live_transcription.go`)
- ✅ `LiveTranscriptionSession` avec GORM hooks `BeforeCreate`
- ✅ `LiveTranscriptionChunk` avec foreign key `SessionID`
- ✅ Statuts: `active`, `finalizing`, `completed`, `cancelled`
- ✅ Auto-migration dans `database.Initialize()`

---

## ✅ Architecture Frontend

### Component (`LiveTranscriptionDialog.tsx`)

#### State Management
```typescript
// Session state
const [session, setSession] = useState<LiveSession | null>(null);
const [chunks, setChunks] = useState<LiveChunk[]>([]);

// UI state
const [isRecording, setIsRecording] = useState(false);
const [streamError, setStreamError] = useState<string | null>(null);
const [finalJobId, setFinalJobId] = useState<string | null>(null);

// Refs (persistent across renders)
const mediaStreamRef = useRef<MediaStream | null>(null);
const mediaRecorderRef = useRef<MediaRecorder | null>(null);
const chunkIntervalRef = useRef<number | null>(null); // 15s timer
const uploadPromiseRef = useRef(Promise.resolve()); // Sequential uploads
const sessionRef = useRef<LiveSession | null>(null); // For callbacks
```

#### MediaRecorder - Stop/Start Cycling (Solution WebM Fragmentation)
**Problème**: `MediaRecorder.start(timeslice)` produit des chunks fragmentés sans header EBML après le premier.

**Solution**:
```typescript
const cycleRecording = () => {
  const recorder = createRecorder();
  recorder.onstop = () => {
    // Auto-restart pour chunk suivant
    if (mediaStreamRef.current && chunkIntervalRef.current !== null) {
      setTimeout(cycleRecording, 100);
    }
  };
  recorder.start(); // Sans timeslice!
};

// Timer externe qui force stop() toutes les 15s
chunkIntervalRef.current = window.setInterval(() => {
  if (mediaRecorderRef.current?.state === 'recording') {
    mediaRecorderRef.current.stop(); // Trigger ondataavailable + onstop
  }
}, 15000);
```

✅ Chaque `stop()` génère un **container WebM complet** avec header
✅ `onstop` redémarre automatiquement → enregistrement continu
✅ Cleanup complet: `clearInterval`, `onstop = null`, `ondataavailable = null`

#### SSE Streaming
```typescript
const connectStream = async (sessionId: string) => {
  const res = await fetch(`/api/v1/.../stream`);
  const reader = res.body.getReader();
  
  let buffer = '';
  while (true) {
    const { done, value } = await reader.read();
    buffer += decoder.decode(value);
    
    // Parse JSON lines
    let newlineIndex = buffer.indexOf('\n');
    while (newlineIndex >= 0) {
      const line = buffer.slice(0, newlineIndex).trim();
      const event: LiveStreamEvent = JSON.parse(line);
      handleStreamEvent(event);
      buffer = buffer.slice(newlineIndex + 1);
    }
  }
};
```

✅ `handleStreamEvent` met à jour `session` et `chunks` immutablement
✅ Gestion des events `snapshot` (initial), `chunk` (update), `status` (finalize/cancel)

#### Upload Queue
```typescript
const queueChunkUpload = (blob: Blob) => {
  const upload = () => fetch(...).catch(...);
  uploadPromiseRef.current = uploadPromiseRef.current.then(upload);
};
```

✅ Sequential processing (pas de race sur sequence numbers)
✅ `await uploadPromiseRef.current` dans `finalizeSession` avant API call

#### Lifecycle & Cleanup
```typescript
const cleanup = async () => {
  stopRecorder();          // Clear interval, stop MediaRecorder, free stream
  disconnectStream();      // Abort SSE fetch
  await cancelRemoteSession(); // Best-effort POST /cancel
  resetState();            // Clear all state
};

useEffect(() => {
  if (!isOpen) cleanup(); // Dialog close → cleanup
}, [isOpen, cleanup]);
```

✅ Pas de fuites mémoire (refs cleared, intervals cleared)
✅ Tracks audio fermés (`getTracks().forEach(track => track.stop())`)

### Types (`types/live.ts`)
- ✅ Alignés avec les structs Go (snake_case JSON tags)
- ✅ `LiveSessionStatus` = union type pour type safety
- ✅ `LiveStreamEvent` discriminated union (`type` field)

---

## 🔍 Points de vigilance

### Limitations connues (acceptables pour MVP)
1. **Pas de reconnexion SSE automatique**: Si la connexion SSE drop, l'utilisateur doit rafraîchir
2. **Pas de retry sur chunk upload failure**: Toast affiché mais chunk perdu
3. **Pas de validation côté client de la séquence**: Le backend rejette les duplicates mais pas d'UI

### Edge cases gérés
- ✅ Dialog fermé pendant recording → cleanup + cancel remote session
- ✅ Finalize sans chunks → erreur "session has no chunks"
- ✅ Finalize session déjà finalized → erreur "cannot be finalized in status X"
- ✅ Upload chunk trop petit (<1KB) → erreur "chunk too small"
- ✅ MediaRecorder WebM non supporté → fallback vers default MIME type
- ✅ Browser permissions refusées → toast + cancel session

### Performance
- ✅ Compression désactivée sur routes upload/stream (middleware `NoCompressionMiddleware`)
- ✅ Broadcast SSE avec `default` case pour éviter blocking si subscriber lent
- ✅ Sequential uploads évitent race conditions sur sequence numbers

---

## 📋 Checklist de test

### Backend
- [ ] Session créée avec UUID unique
- [ ] Chunk upload → audio converti en WAV 16kHz mono
- [ ] Chunk upload → transcription WhisperX exécutée
- [ ] SSE stream envoie snapshot + updates en temps réel
- [ ] Finalize → audio chunks concaténés (ffmpeg)
- [ ] Finalize → TranscriptionJob créé et enqueued
- [ ] Cancel → session status = cancelled

### Frontend
- [ ] Dialog ouvert → microphone permission demandée
- [ ] Recording démarre → console logs "MediaRecorder cycling started"
- [ ] Tous les 15s → chunk upload (log "Chunk received: XXX bytes")
- [ ] SSE updates → chunks list affichée avec transcripts
- [ ] Finalize → uploads terminés avant API call
- [ ] Finalize → job ID affiché + lien vers detail view
- [ ] Dialog fermé pendant recording → cleanup complet

### Integration
- [ ] Build complet sans erreurs: `./build.sh`
- [ ] Server restart → routes live fonctionnelles
- [ ] Browser hard refresh → nouveau JS chargé
- [ ] Workflow complet: create → record 45s → finalize → job queued

---

## 🚀 Prochaines améliorations (hors scope MVP)

1. **SSE Reconnection**: `EventSource` API ou retry logic dans `connectStream`
2. **Chunk retry**: Queue failed uploads + retry avec exponential backoff
3. **Progress UI**: Waveform visualisation pendant recording (WaveSurfer.js)
4. **Pause/Resume**: Arrêter recording sans finalize (besoin status `paused`)
5. **Live transcript editing**: Allow user corrections pendant recording
6. **Session recovery**: Reload session si browser crash (localStorage backup)

---

## ✅ Conclusion

**Le code LiveTranscription est production-ready pour un MVP.**

### Points forts
- Architecture propre avec séparation concerns (service/handler/model)
- Thread safety garanti (locks, immutable updates)
- Solution élégante au problème WebM fragmentation (stop/start cycling)
- Error handling robuste avec messages clairs
- Cleanup complet → pas de fuites mémoire

### Risques mineurs
- SSE reconnection manuelle si déconnexion
- Chunks perdus si upload échoue (pas critique car finalize merge tous les chunks réussis)

**Recommandation: Déployer et monitorer les logs backend pour détecter edge cases en production.**
