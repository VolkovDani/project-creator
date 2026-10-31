'use client';
import Analysis from '../Analysis';
import {useProject} from '../ProjectShell';
export default function AnalysisPage(){const {project,locale}=useProject();return <Analysis key={project.id} projectId={project.id} locale={locale}/>}
