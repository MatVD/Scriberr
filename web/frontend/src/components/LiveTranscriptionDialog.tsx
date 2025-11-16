import { useCallback, useEffect, useRef, useState } from 'react';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { useAuth } from '@/contexts/AuthContext';
import { useRouter } from '@/contexts/RouterContext';
import { useToast } from '@/components/ui/toast';
import type { LiveSession, LiveStreamEvent } from '@/types/live';
import { AlertCircle, Pause, PlayCircle, Radio, StopCircle } from 'lucide-react';

interface LiveTranscriptionDialogProps {
  isOpen: boolean;
  onClose: () => void;
  onSessionComplete?: () => void;
}

interface FinalizeResponse {
  session: LiveSession;
  job: {
    id: string;
    title?: string;
    status: string;
  };
}

export function LiveTranscriptionDialog({ isOpen, onClose, onSessionComplete }: LiveTranscriptionDialogProps) {
  const { getAuthHeaders } = useAuth();
  const { navigate } = useRouter();
  const { toast } = useToast();

  const [title, setTitle] = useState('');
  const [session, setSession] = useState<LiveSession | null>(null);
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
    setSession(null);
    setStreamError(null);
    setFinalJobId(null);
    setPendingAction(null);
  }, []);

  const stopRecorder = useCallback(() => {
    return new Promise<void>((resolve) => {
      if (chunkIntervalRef.current !== null) {
        clearInterval(chunkIntervalRef.current);
        chunkIntervalRef.current = null;
      }
      
      if (!mediaRecorderRef.current) {
        if (mediaStreamRef.current) {
          mediaStreamRef.current.getTracks().forEach(track => track.stop());
          mediaStreamRef.current = null;
        }
        setIsRecording(false);
        resolve();
        return;
      }

      // Set up one-time handler to wait for final chunk
      const currentRecorder = mediaRecorderRef.current;
      
      currentRecorder.onstop = () => {
        // Don't restart (interval is cleared)
        if (mediaStreamRef.current) {
          mediaStreamRef.current.getTracks().forEach(track => track.stop());
          mediaStreamRef.current = null;
        }
        mediaRecorderRef.current = null;
        setIsRecording(false);
        resolve();
      };

      try {
        currentRecorder.stop();
      } catch (err) {
        console.warn('Failed to stop media recorder', err);
        currentRecorder.ondataavailable = null;
        currentRecorder.onerror = null;
        currentRecorder.onstop = null;
        mediaRecorderRef.current = null;
        if (mediaStreamRef.current) {
          mediaStreamRef.current.getTracks().forEach(track => track.stop());
          mediaStreamRef.current = null;
        }
        setIsRecording(false);
        resolve();
      }
    });
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
    await stopRecorder();
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
    
    // Stop recording and wait for final chunk to be emitted
    await stopRecorder();
    
    // Wait for all pending uploads to complete (including the final chunk)
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
      
      // Trigger refresh and redirect to homepage after a short delay
      onSessionComplete?.();
      setTimeout(() => {
        handleClose();
        navigate({ path: 'home' });
      }, 2000);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Could not finalize session';
      toast({ title: 'Finalize failed', description: message });
    } finally {
      setPendingAction(null);
    }
  };

  const handleClose = async () => {
    await cleanup();
    onClose();
  };

  const hasSession = !!session;

  const [recordingTime, setRecordingTime] = useState(0);

  useEffect(() => {
    if (!session || !isRecording) return;

    const startTime = new Date(session.created_at).getTime();
    const interval = setInterval(() => {
      const elapsed = Date.now() - startTime;
      setRecordingTime(elapsed);
    }, 100);

    return () => clearInterval(interval);
  }, [session, isRecording]);

  const formatTime = (timeMs: number) => {
    const minutes = Math.floor(timeMs / 60000);
    const seconds = Math.floor((timeMs % 60000) / 1000);
    return `${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`;
  };

  return (
    <Dialog open={isOpen} onOpenChange={() => handleClose()}>
      <DialogContent className="sm:max-w-[600px] bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700">
        <DialogHeader>
          <DialogTitle className="text-gray-900 dark:text-gray-100 text-xl font-semibold">
            Live Transcription
          </DialogTitle>
          <DialogDescription className="text-gray-600 dark:text-gray-400">
            Stream audio directly from your microphone for real-time transcription.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6 py-4">
          {/* Title Input */}
          <div className="space-y-2">
            <label className="text-sm font-medium text-gray-700 dark:text-gray-300">
              Session Title (Optional)
            </label>
            <Input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Enter a title for your session..."
              className="bg-white dark:bg-gray-800 border-gray-300 dark:border-gray-600 text-gray-900 dark:text-gray-100"
              disabled={hasSession}
            />
          </div>

          {/* Recording Time */}
          {hasSession && (
            <div className="text-center">
              <div className="text-3xl font-mono font-bold text-gray-900 dark:text-gray-100 mb-2">
                {formatTime(recordingTime)}
              </div>
              <div className="flex items-center justify-center gap-2 text-sm text-gray-600 dark:text-gray-400">
                {isRecording && (
                  <div className="h-2 w-2 bg-red-500 rounded-full animate-pulse"></div>
                )}
                <span>
                  {isRecording ? 'Recording...' : session?.status === 'completed' ? 'Completed' : 'Paused'}
                </span>
              </div>
              {isRecording && (
                <div className="text-xs text-blue-600 dark:text-blue-400 mt-1">
                  Recording continues even if you switch tabs
                </div>
              )}
              {streamError && (
                <div className="flex items-center justify-center gap-2 text-amber-600 text-xs mt-2">
                  <AlertCircle className="h-4 w-4" />
                  {streamError}
                </div>
              )}
            </div>
          )}

          {/* Waveform Container */}
          {hasSession && (
            <div className="relative">
              <div className="w-full rounded-lg p-4 bg-gray-50 dark:bg-gray-800/50 min-h-[120px] flex items-center justify-center">
                {pendingAction === 'finalize' ? (
                  <div className="text-center space-y-3">
                    <div className="flex items-center justify-center gap-2">
                      <div className="h-2 w-2 bg-blue-500 rounded-full animate-pulse"></div>
                      <div className="h-2 w-2 bg-blue-500 rounded-full animate-pulse" style={{ animationDelay: '0.2s' }}></div>
                      <div className="h-2 w-2 bg-blue-500 rounded-full animate-pulse" style={{ animationDelay: '0.4s' }}></div>
                    </div>
                    <div className="text-gray-700 dark:text-gray-300 font-medium">
                      Processing your transcription...
                    </div>
                    <div className="text-gray-500 dark:text-gray-400 text-sm">
                      This will take a few moments
                    </div>
                  </div>
                ) : isRecording ? (
                  <div className="w-full flex items-center gap-1 h-16 justify-center">
                    {[...Array(20)].map((_, i) => (
                      <div
                        key={i}
                        className="w-1 bg-purple-500 rounded-full"
                        style={{
                          height: '30%',
                          animation: `waveform 1s ease-in-out infinite`,
                          animationDelay: `${i * 0.07}s`,
                        }}
                      />
                    ))}
                  </div>
                ) : (
                  <div className="text-gray-400 dark:text-gray-500 text-sm text-center">
                    <Radio className="h-8 w-8 mx-auto mb-2 opacity-50" />
                    <div>Audio waveform will appear here during recording</div>
                  </div>
                )}
              </div>
              <style>{`
                @keyframes waveform {
                  0%, 100% { height: 2%; opacity: 0.3; }
                  50% { height: 100%; opacity: 1; }
                }
              `}</style>
            </div>
          )}

          {/* Final Job Link
          {finalJobId && (
            <div className="flex items-center justify-between rounded-lg border border-green-200 dark:border-green-800 bg-green-50 dark:bg-green-900/20 p-3 text-sm">
              <div className="text-green-800 dark:text-green-200">
                ✓ Transcription job queued successfully
              </div>
              <Button 
                variant="link" 
                className="p-0 text-green-700 dark:text-green-300" 
                onClick={() => {
                  handleClose();
                  navigate({ path: 'audio-detail', params: { id: finalJobId } });
                }}
              >
                View job <ExternalLink className="h-4 w-4 ml-1" />
              </Button>
            </div>
          )} */}

          {/* Recording Controls */}
          <div className="flex justify-center gap-4">
            {!hasSession && (
              <Button
                onClick={startSession}
                disabled={isStarting}
                size="lg"
                className="bg-red-500 hover:bg-red-600 text-white px-8 py-3 rounded-xl font-medium transition-all duration-300 hover:scale-105"
              >
                {isStarting ? (
                  <>Preparing...</>
                ) : (
                  <>
                    <Radio className="h-5 w-5 mr-2" />
                    Start Live Session
                  </>
                )}
              </Button>
            )}

            {hasSession && session?.status === 'active' && (
              <>
                {isRecording && (
                  <Button
                    onClick={stopRecorder}
                    size="lg"
                    variant="outline"
                    className="border-gray-300 dark:border-gray-600 hover:bg-gray-100 dark:hover:bg-gray-700 px-6 py-3 rounded-xl"
                  >
                    <Pause className="h-5 w-5 mr-2" />
                    Pause
                  </Button>
                )}
                
                {!isRecording && (
                  <Button
                    onClick={() => startRecorder(session.id)}
                    size="lg"
                    variant="outline"
                    className="border-gray-300 dark:border-gray-600 hover:bg-gray-100 dark:hover:bg-gray-700 px-6 py-3 rounded-xl"
                  >
                    <PlayCircle className="h-5 w-5 mr-2" />
                    Resume
                  </Button>
                )}

                <Button
                  onClick={finalizeSession}
                  disabled={pendingAction === 'finalize'}
                  size="lg"
                  className={`bg-blue-500 hover:bg-blue-600 text-white px-8 py-3 rounded-xl font-medium transition-all duration-300 ${
                    pendingAction === 'finalize' ? 'animate-pulse' : 'hover:scale-105'
                  }`}
                >
                  {pendingAction === 'finalize' ? (
                    <>Finalizing...</>
                  ) : (
                    <>
                      <StopCircle className="h-5 w-5 mr-2" />
                      Finalize & Upload
                    </>
                  )}
                </Button>
              </>
            )}

            {hasSession && session?.status !== 'active' && !finalJobId && (
              <div className="text-center text-sm text-gray-500">
                Session {session?.status}
              </div>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
