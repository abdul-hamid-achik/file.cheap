-- Enterprise inquiries table for tracking contact requests
CREATE TABLE enterprise_inquiries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    company_name VARCHAR(255) NOT NULL,
    contact_name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL,
    phone VARCHAR(50),
    company_size VARCHAR(20) NOT NULL,
    estimated_usage VARCHAR(50) NOT NULL,
    message TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    admin_notes TEXT,
    processed_at TIMESTAMPTZ,
    processed_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_enterprise_inquiries_user_id ON enterprise_inquiries(user_id);
CREATE INDEX idx_enterprise_inquiries_status ON enterprise_inquiries(status);
CREATE INDEX idx_enterprise_inquiries_created_at ON enterprise_inquiries(created_at DESC);
