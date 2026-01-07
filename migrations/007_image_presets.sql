-- Migration: Update variant_type enum with industry-standard sizes and social media presets
-- This migration removes old variant types (small, medium, large) and adds new ones

BEGIN;

-- Delete any existing variants (clean slate since no users exist)
DELETE FROM file_variants;

-- Drop and recreate the enum with new values
ALTER TYPE variant_type RENAME TO variant_type_old;

CREATE TYPE variant_type AS ENUM (
    'thumbnail',
    'sm',
    'md',
    'lg',
    'xl',
    'og',
    'twitter',
    'instagram_square',
    'instagram_portrait',
    'instagram_story',
    'webp',
    'watermarked',
    'optimized',
    'pdf_preview'
);

-- Update the column to use new type
ALTER TABLE file_variants 
    ALTER COLUMN variant_type TYPE variant_type 
    USING variant_type::text::variant_type;

-- Drop old enum
DROP TYPE variant_type_old;

COMMIT;
