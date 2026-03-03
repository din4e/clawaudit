import { configureStore, combineReducers } from '@reduxjs/toolkit';
import { apiSlice } from './services/api';
import scansReducer from './slices/scansSlice';
import uiReducer from './slices/uiSlice';
import themeReducer from './slices/themeSlice';

// Combine reducers
const rootReducer = combineReducers({
  [apiSlice.reducerPath]: apiSlice.reducer,
  scans: scansReducer,
  ui: uiReducer,
  theme: themeReducer,
});

// Create store
export const makeStore = () => {
  return configureStore({
    reducer: rootReducer,
    middleware: (getDefaultMiddleware) =>
      getDefaultMiddleware().concat(apiSlice.middleware),
    devTools: process.env.NODE_ENV !== 'production',
  });
};

// Infer types
export type RootState = ReturnType<typeof rootReducer>;
export type AppStore = ReturnType<typeof makeStore>;
export type AppDispatch = AppStore['dispatch'];

// Create store instance
export const store = makeStore();
