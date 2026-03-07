import { Router, Request, Response } from 'express';
import { v4 as uuidv4 } from 'uuid';
import { persistence } from '../services/persistenceManager';
import { requireAuth } from '../middleware/auth';
import { KeyEntry } from '../types';

const router = Router();
router.use(requireAuth);

router.get('/', (_req: Request, res: Response) => {
  try {
    const config = persistence.loadKeys();
    // Never expose private_key_path to the client — only id, type, description, encrypted
    const safe = config.keys.map(k => ({
      id: k.id,
      type: k.type,
      encrypted: k.encrypted,
      description: k.description,
    }));
    res.json(safe);
  } catch {
    res.status(500).json({ error: 'Failed to load keys' });
  }
});

router.post('/', (req: Request, res: Response) => {
  const { private_key_path, type, encrypted, description, id } = req.body as Partial<KeyEntry>;

  if (!private_key_path) {
    res.status(400).json({ error: 'private_key_path is required' });
    return;
  }

  try {
    const config = persistence.loadKeys();
    const key: KeyEntry = {
      id: id || uuidv4(),
      type: type || 'rsa',
      private_key_path,
      encrypted: encrypted ?? false,
      description: description || '',
    };
    config.keys.push(key);
    persistence.saveKeys(config);
    res.status(201).json({ id: key.id, type: key.type, encrypted: key.encrypted, description: key.description });
  } catch {
    res.status(500).json({ error: 'Failed to save key' });
  }
});

router.delete('/:id', (req: Request, res: Response) => {
  const { id } = req.params;
  try {
    const config = persistence.loadKeys();
    const idx = config.keys.findIndex(k => k.id === id);
    if (idx < 0) {
      res.status(404).json({ error: 'Key not found' });
      return;
    }
    config.keys.splice(idx, 1);
    persistence.saveKeys(config);
    res.status(204).send();
  } catch {
    res.status(500).json({ error: 'Failed to delete key' });
  }
});

export default router;
