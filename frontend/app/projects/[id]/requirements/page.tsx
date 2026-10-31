'use client';
import {Suspense} from 'react';
import Requirements from '../Requirements';
import {useProject} from '../ProjectShell';
export default function RequirementsPage(){const {project,locale}=useProject();return <Suspense><Requirements key={project.id} projectId={project.id} locale={locale}/></Suspense>}
