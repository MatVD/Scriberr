import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { useAuth } from '@/contexts/AuthContext';
import { useRouter } from '@/contexts/RouterContext';
import { useToast } from '@/components/ui/toast';
import type { LiveChunk, LiveSession, LiveSessionStatus, LiveStreamEvent } from '@/types/live';
import { AlertCircle, ExternalLink, Mic, Pause, PlayCircle, Radio, StopCircle } from 'lucide-react';

interface LiveTranscriptionDialogProps {
  isOpen: boolean;
  onClose: () => void;
}

interface FinalizeResponse {
  session: LiveSession;
  job: {
    id: string;
    title?: string;
    status: string;
  };
}

export function LiveTranscriptionDialog({ isOpen, onClose }: LiveTranscriptionDialogProps) {
  const { getAuthHeaders } = useAuth();
  const { navigate } = useRouter();
  const { toast } = useToast();

  const [title, setTitle] = useState('');
  const [session, setSession] = useState<LiveSession | null>(null);
  const [chunks, setChunks] = useState<LiveChunk[]>([]);
  const [isStarting, setIsStarting] = useState(false);
  const [isRecording, setIsRecording] = useState(false);
  const [streamError, setStreamError] = useState<string | null>(null);
  const [finalJobId, setFinalJobId] = useState<string | null>(null);
  const [pendingAction, setPendingAction] = useState<'finalize' | 'cancel' | null>(null);

  const mediaStreamRef = useRef<MediaStream | null>(null);
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const streamAbortRef = useRef<AbortController | null>(null);
  const chunkSequenceRef = useRef(0);
  const uploadPromiseRef = useRef(Promise.resolve());
  const startTimestampRef = useRef<number | null>(null);
  const lastChunkEndRef = useRef(0);
  const sessionRef = useRef<LiveSession | null>(null);
  const chunkIntervalRef = useRef<number | null>(null);

  useEffect(() => {
    sessionRef.current = session;
  }, [session]);

  const resetState = useCallback(() => {
    chunkSequenceRef.current = 0;
    startTimestampRef.current = null;
    lastChunkEndRef.current = 0;
    setChunks([]);
    setSession(null);
    setStreamError(null);
    setFinalJobId(null);
    setPendingAction(null);
  }, []);

  const stopRecorder = useCallback(() => {
    if (chunkIntervalRef.current !== null) {
      clearInterval(chunkIntervalRef.current);
      chunkIntervalRef.current = null;
    }
    if (mediaRecorderRef.current) {
      try {
        mediaRecorderRef.current.stop();
      } catch (err) {
        console.warn('Failed to stop media recorder', err);
      }
      mediaRecorderRef.current.ondataavailable = null;
      mediaRecorderRef.current.onerror = null;
      mediaRecorderRef.current.onstop = null;
      mediaRecorderRef.current = null;
    }
    if (mediaStreamRef.current) {
      mediaStreamRef.current.getTracks().forEach(track => track.stop());
      mediaStreamRef.current = null;
    }
    setIsRecording(false);
  }, []);

  const disconnectStream = useCallback(() => {
    if (streamAbortRef.current) {
      streamAbortRef.current.abort();
      streamAbortRef.current = null;
    }
  }, []);

  const cancelRemoteSession = useCallback(async () => {
    const active = sessionRef.current;
    if (!active || (active.status !== 'active' && active.status !== 'finalizing')) {
      return;
    }
    try {
      await fetch(`/api/v1/transcription/live/sessions/${active.id}/cancel`, {
        method: 'POST',
        headers: {
          ...getAuthHeaders(),
        },
      });
    } catch {
      // Best-effort cancellation
    }
  }, [getAuthHeaders]);

  const cleanup = useCallback(async () => {
    stopRecorder();
    disconnectStream();
    await cancelRemoteSession();
    resetState();
  }, [stopRecorder, disconnectStream, cancelRemoteSession, resetState]);

  useEffect(() => {
    if (!isOpen) {
      cleanup();
    }
  }, [isOpen, cleanup]);

  const queueChunkUpload = useCallback((blob: Blob) => {
    const currentSession = sessionRef.current;
    if (!currentSession) return;

    const seq = ++chunkSequenceRef.current;
    const now = performance.now();
    if (!startTimestampRef.current) {
      startTimestampRef.current = now;
    }
    const elapsedSeconds = (now - startTimestampRef.current) / 1000;
    const startOffset = lastChunkEndRef.current;
    const endOffset = elapsedSeconds;
    lastChunkEndRef.current = endOffset;

    const formData = new FormData();
    formData.append('chunk', blob, `chunk-${seq}.webm`);
    formData.append('sequence', seq.toString());
    formData.append('start_offset', startOffset.toFixed(3));
    formData.append('end_offset', endOffset.toFixed(3));

    const headers = new Headers();
    const authHeaders = getAuthHeaders();
    Object.entries(authHeaders).forEach(([key, value]) => headers.set(key, value));

    const upload = () =>
      fetch(`/api/v1/transcription/live/sessions/${currentSession.id}/chunks`, {
        method: 'POST',
        headers,
        body: formData,
      })
        .catch(() => {
          toast({
            title: 'Chunk upload failed',
            description: 'A live chunk failed to upload. Trying to continue.',
          });
        })
        .then(() => undefined);

    uploadPromiseRef.current = uploadPromiseRef.current.then(upload);
  }, [getAuthHeaders, toast]);

  const handleStreamEvent = useCallback((event: LiveStreamEvent) => {
    setSession(prev => {
      const next: LiveSession = prev ? { ...prev } : {
        id: event.session_id,
        status: event.session_status,
        chunk_count: 0,
        last_sequence: 0,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      } as LiveSession;
      next.status = event.session_status;
      if (event.title !== undefined) {
        next.title = event.title || undefined;
      }
      if (event.final_job_id) {
        next.final_job_id = event.final_job_id;
        setFinalJobId(event.final_job_id);
      }
      if (event.accumulated_text !== undefined) {
        next.accumulated_transcript = event.accumulated_text || undefined;
      }
      next.updated_at = event.timestamp;
      return next;
    });

    if (event.type === 'snapshot' && event.chunks) {
      setChunks(event.chunks);
    }
    if (event.type === 'chunk' && event.chunk) {
      setChunks(prev => {
        const filtered = prev.filter(chunk => chunk.sequence !== event.chunk!.sequence);
        return [...filtered, event.chunk!].sort((a, b) => a.sequence - b.sequence);
      });
    }
  }, []);

  const connectStream = useCallback(async (sessionId: string) => {
    const controller = new AbortController();
    streamAbortRef.current = controller;
    try {
      const res = await fetch(`/api/v1/transcription/live/sessions/${sessionId}/stream`, {
        headers: {
          ...getAuthHeaders(),
        },
        signal: controller.signal,
      });
      if (!res.body) {
        setStreamError('Server did not provide a stream.');
        return;
      }
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        let newlineIndex = buffer.indexOf('\n');
        while (newlineIndex >= 0) {
          const line = buffer.slice(0, newlineIndex).trim();
          buffer = buffer.slice(newlineIndex + 1);
          if (line) {
            try {
              const event: LiveStreamEvent = JSON.parse(line);
              handleStreamEvent(event);
            } catch (err) {
              console.error('Failed to parse live event', err);
            }
          }
          newlineIndex = buffer.indexOf('\n');
        }
      }
    } catch (err) {
      if (!controller.signal.aborted) {
        console.error('Live stream error', err);
        setStreamError('Connection lost. You can keep recording, but data may be delayed.');
      }
    }
  }, [getAuthHeaders, handleStreamEvent]);

  const startRecorder = useCallback(async (sessionId: string) => {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ 
        audio: { 
          channelCount: 1,
          sampleRate: 16000,
          echoCancellation: true,
          noiseSuppression: true 
        } 
      });
      mediaStreamRef.current = stream;
      
      // Try webm first, fallback to any supported format
      let mimeType = 'audio/webm;codecs=opus';
      if (!MediaRecorder.isTypeSupported(mimeType)) {
        mimeType = 'audio/webm';
        if (!MediaRecorder.isTypeSupported(mimeType)) {
          console.warn('WebM not supported, using default MIME type');
          mimeType = '';
        }
      }
      
      const createRecorder = () => {
        const options = mimeType ? { mimeType } : {};
        const recorder = new MediaRecorder(stream, options);
        
        recorder.ondataavailable = event => {
          if (event.data && event.data.size > 0) {
            console.log(`Chunk received: ${event.data.size} bytes, type: ${event.data.type}`);
            queueChunkUpload(event.data);
          }
        };
        
        recorder.onerror = (err) => {
          console.error('MediaRecorder error:', err);
          toast({ title: 'Recording error', description: 'Audio capture failed' });
        };
        
        return recorder;
      };

      // Use stop/start cycling to force complete WebM containers with headers
      const cycleRecording = () => {
        if (!mediaStreamRef.current) return;
        
        const recorder = createRecorder();
        mediaRecorderRef.current = recorder;
        
        recorder.onstop = () => {
          // Auto-restart after stop to get next chunk
          if (mediaStreamRef.current && chunkIntervalRef.current !== null) {
            setTimeout(cycleRecording, 100);
          }
        };
        
        recorder.start();
      };

      // Start first recording cycle
      cycleRecording();
      
      // Schedule stop every 15 seconds to get complete chunks
      chunkIntervalRef.current = window.setInterval(() => {
        if (mediaRecorderRef.current && mediaRecorderRef.current.state === 'recording') {
          mediaRecorderRef.current.stop();
        }
      }, 15000);
      
      console.log(`MediaRecorder cycling started with MIME type: ${mimeType}`);
      setIsRecording(true);
      connectStream(sessionId);
    } catch (error) {
      console.error('Failed to start recorder', error);
      toast({ title: 'Microphone access denied', description: 'Please allow microphone access to stream audio.' });
      await cancelRemoteSession();
    }
  }, [queueChunkUpload, connectStream, toast, cancelRemoteSession]);

  const startSession = async () => {
    setIsStarting(true);
    try {
      const response = await fetch('/api/v1/transcription/live/sessions', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...getAuthHeaders(),
        },
        body: JSON.stringify({
          title: title || undefined,
        }),
      });
      if (!response.ok) {
        const message = await response.text();
        throw new Error(message || 'Failed to create live session');
      }
      const sessionData: LiveSession = await response.json();
      setSession(sessionData);
      toast({ title: 'Live transcription started' });
      await startRecorder(sessionData.id);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      console.error('Failed to start live session', error);
      toast({ title: 'Failed to start live session', description: message });
    } finally {
      setIsStarting(false);
    }
  };

  const finalizeSession = async () => {
    if (!session) return;
    setPendingAction('finalize');
    stopRecorder();
    
    // Wait for all pending uploads to complete
    await uploadPromiseRef.current;
    
    try {
      const response = await fetch(`/api/v1/transcription/live/sessions/${session.id}/finalize`, {
        method: 'POST',
        headers: {
          ...getAuthHeaders(),
        },
      });
      if (!response.ok) {
        const message = await response.text();
        throw new Error(message || 'Failed to finalize session');
      }
      const data: FinalizeResponse = await response.json();
      setSession(data.session);
      setFinalJobId(data.job.id);
      toast({ title: 'Final job queued', description: 'You can monitor progress from the Jobs table.' });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Could not finalize session';
      toast({ title: 'Finalize failed', description: message });
    } finally {
      setPendingAction(null);
    }
  };

  const cancelSession = async () => {
    if (!session) {
      await cleanup();
      onClose();
      return;
    }
    setPendingAction('cancel');
    stopRecorder();
    try {
      await fetch(`/api/v1/transcription/live/sessions/${session.id}/cancel`, {
        method: 'POST',
        headers: {
          ...getAuthHeaders(),
        },
      });
      setSession(prev => (prev ? { ...prev, status: 'cancelled' as LiveSessionStatus } : prev));
      toast({ title: 'Session cancelled' });
    } finally {
      setPendingAction(null);
    }
  };

  const handleClose = async () => {
    await cleanup();
    onClose();
  };

  const statusBadge = useMemo(() => {
    if (!session) return null;
    const color = session.status === 'active'
      ? 'bg-green-100 text-green-800'
      : session.status === 'finalizing'
        ? 'bg-amber-100 text-amber-800'
        : session.status === 'completed'
          ? 'bg-blue-100 text-blue-800'
          : 'bg-red-100 text-red-800';
    return (
      <span className={`text-xs px-2 py-1 rounded-full font-medium ${color}`}>
        {session.status.toUpperCase()}
      </span>
    );
  }, [session]);

  const hasSession = !!session;

  return (
    <Dialog open={isOpen} onOpenChange={() => handleClose()}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Radio className="h-5 w-5 text-blue-500" />
            Live Transcription
          </DialogTitle>
          <DialogDescription>
            Stream audio directly to Scriberr and receive rolling transcripts while the meeting unfolds.
          </DialogDescription>
        </DialogHeader>

        {!hasSession && (
          <div className="space-y-4">
            <Input
              placeholder="Meeting title (optional)"
              value={title}
              onChange={e => setTitle(e.target.value)}
            />
            <Button onClick={startSession} disabled={isStarting} className="w-full">
              {isStarting ? 'Preparing microphones…' : 'Start live session'}
            </Button>
          </div>
        )}

        {hasSession && (
          <div className="space-y-4">
            <div className="flex items-center justify-between gap-2">
              <div>
                <div className="font-semibold text-gray-900 dark:text-gray-100">{session?.title || 'Untitled meeting'}</div>
                <div className="text-xs text-gray-500">Session ID: {session?.id}</div>
              </div>
              {statusBadge}
            </div>

            {streamError && (
              <div className="flex items-center gap-2 text-amber-600 text-sm">
                <AlertCircle className="h-4 w-4" />
                {streamError}
              </div>
            )}

            <div className="p-3 border rounded-lg h-48 overflow-y-auto bg-gray-50 dark:bg-gray-800/50">
              {chunks.length === 0 ? (
                <div className="text-sm text-gray-500 flex items-center gap-2">
                  <Mic className="h-4 w-4" /> Waiting for the first chunk…
                </div>
              ) : (
                chunks.map(chunk => (
                  <div key={chunk.sequence} className="mb-3">
                    <div className="text-xs text-gray-500">Chunk #{chunk.sequence} · {chunk.start_offset.toFixed(1)}s → {chunk.end_offset.toFixed(1)}s</div>
                    <div className="text-sm text-gray-900 dark:text-gray-100">
                      {chunk.text || <span className="text-gray-400">(transcript pending)</span>}
                    </div>
                  </div>
                ))
              )}
            </div>

            {finalJobId && (
              <div className="flex items-center justify-between rounded-lg border p-3 text-sm">
                <div>
                  Final job queued
                  <div className="text-xs text-gray-500">#{finalJobId}</div>
                </div>
                <Button variant="link" className="p-0" onClick={() => {
                  handleClose();
                  navigate({ path: 'audio-detail', params: { id: finalJobId } });
                }}>
                  View job <ExternalLink className="h-4 w-4 ml-1" />
                </Button>
              </div>
            )}

            <div className="flex flex-col sm:flex-row gap-2">
              <Button
                className="flex-1"
                variant={isRecording ? 'destructive' : 'default'}
                disabled={pendingAction !== null}
                onClick={isRecording ? stopRecorder : () => startRecorder(session!.id)}
              >
                {isRecording ? (
                  <span className="flex items-center gap-2"><Pause className="h-4 w-4" /> Pause Capture</span>
                ) : (
                  <span className="flex items-center gap-2"><PlayCircle className="h-4 w-4" /> Resume Capture</span>
                )}
              </Button>
              <Button
                className="flex-1"
                onClick={finalizeSession}
                disabled={pendingAction === 'finalize'}
              >
                <span className="flex items-center gap-2"><StopCircle className="h-4 w-4" />
                  {pendingAction === 'finalize' ? 'Finalizing…' : 'Finalize & Queue Job'}
                </span>
              </Button>
              <Button
                variant="secondary"
                onClick={cancelSession}
                disabled={pendingAction === 'cancel'}
              >
                Cancel
              </Button>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
