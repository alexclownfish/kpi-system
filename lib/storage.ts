const storagePrefix = '__dootask_kpi__';

export const storage = {
  getItem: (key: string) => {
    return localStorage.getItem(storagePrefix + key);
  },
  getJsonItem: <T>(key: string, defaultValue: T): T => {
    const item = storage.getItem(key);
    return item ? JSON.parse(item) : defaultValue;
  },
  setItem: (key: string, value: string) => {
    localStorage.setItem(storagePrefix + key, value);
  },
  removeItem: (key: string) => {
    localStorage.removeItem(storagePrefix + key);
  },
};
